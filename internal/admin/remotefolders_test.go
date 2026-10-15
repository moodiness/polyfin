package admin

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"encoding/pem"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"

	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/localfiles"
	"github.com/moodiness/polyfin/internal/secrets"
)

// The folders of S3 buckets, SFTP servers and rclone remotes are added and
// changed with their fields; their secret keys, passwords, private keys
// and passphrases are stored sealed and never answered, only whether they
// are set. A malformed address, endpoint, key, remote or interval is
// refused with its code.
func TestRemoteFoldersAPI(t *testing.T) {
	t.Parallel()
	box, err := secrets.New(bytes.Repeat([]byte{9}, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	var deps testDeps
	api := newTestAPI(t, 10, func(o *Options, d testDeps) {
		deps = d
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))
		lib := library.New(d.pool, d.addons, d.client, logger, o.Accounts.Settings)
		folders := localfiles.New(d.pool, d.addons, lib, logger, o.Accounts.Settings, box)
		t.Cleanup(folders.Close)
		o.Folders = folders
	})
	admin := api.signedIn("root", true)
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	block, err := ssh.MarshalPrivateKeyWithPassphrase(private, "", []byte("key phrase"))
	if err != nil {
		t.Fatal(err)
	}
	privateKey := string(pem.EncodeToMemory(block))

	// The storages are not reached: an address no server answers at.
	secretsSent := []string{"bucket secret", "key phrase", "rclone secret", privateKey}
	added := map[string]string{}
	for name, body := range map[string]map[string]any{
		"s3": {"name": "Bucket", "path": "s3://media/movies", "kind": "movies", "endpoint": "http://127.0.0.1:9/", "region": "eu-test-1",
			"user": "AKIDTEST", "password": "bucket secret"},
		"sftp": {"name": "Server", "path": "sftp://127.0.0.1:9/srv/movies", "kind": "movies", "user": "reader", "privateKey": privateKey,
			"passphrase": "key phrase", "pollMinutes": 5},
		"rclone": {"name": "Remote", "path": "rclone://127.0.0.1:9/movies", "kind": "movies", "rcloneRemote": "media", "user": "rc",
			"password": "rclone secret"},
	} {
		status, answer, _ := admin.call(http.MethodPost, "/scopes/shared/folders", body)
		encoded, _ := json.Marshal(answer)
		for _, secret := range secretsSent {
			if strings.Contains(string(encoded), strings.TrimSpace(secret)) || strings.Contains(string(encoded), "PRIVATE KEY") {
				t.Errorf("%s: a secret answered: %s", name, encoded)
			}
		}
		var addon struct {
			ID     string `json:"id"`
			Folder struct {
				Share         string `json:"share"`
				Endpoint      string `json:"endpoint"`
				Region        string `json:"region"`
				RcloneRemote  string `json:"rcloneRemote"`
				PasswordSet   bool   `json:"passwordSet"`
				PrivateKeySet bool   `json:"privateKeySet"`
				PassphraseSet bool   `json:"passphraseSet"`
				PollMinutes   *int   `json:"pollMinutes"`
			} `json:"folder"`
		}
		if err := json.Unmarshal(encoded, &addon); err != nil || status != http.StatusCreated || addon.Folder.Share != name {
			t.Fatalf("%s: %d %s", name, status, encoded)
		}
		added[name] = addon.ID
		f := addon.Folder
		switch name {
		case "s3":
			if f.Endpoint != "http://127.0.0.1:9" || f.Region != "eu-test-1" || !f.PasswordSet || f.PollMinutes == nil || *f.PollMinutes != 30 {
				t.Errorf("s3: %+v", f)
			}
		case "sftp":
			if !f.PrivateKeySet || !f.PassphraseSet || f.PasswordSet || f.PollMinutes == nil || *f.PollMinutes != 5 {
				t.Errorf("sftp: %+v", f)
			}
		case "rclone":
			if f.RcloneRemote != "media" || !f.PasswordSet || f.PollMinutes == nil || *f.PollMinutes != 2 {
				t.Errorf("rclone: %+v", f)
			}
		}
	}
	rows, err := deps.pool.Query(t.Context(), "SELECT share_password, share_private_key, share_passphrase FROM local_folders")
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var values [3]string
		if err := rows.Scan(&values[0], &values[1], &values[2]); err != nil {
			t.Fatal(err)
		}
		for _, value := range values {
			if value != "" && !secrets.Sealed(value) {
				t.Errorf("a secret stored unsealed: %.12s…", value)
			}
		}
	}
	rows.Close()

	// A change keeps what it leaves out; an empty passphrase removes it,
	// and the key it opened no longer opens.
	status, answer, _ := admin.call(http.MethodPatch, "/scopes/shared/folders/"+added["sftp"], map[string]any{"pollMinutes": 0})
	if folder, _ := answer["folder"].(map[string]any); status != http.StatusOK || folder["pollMinutes"] != float64(0) ||
		folder["privateKeySet"] != true {
		t.Errorf("a change: %d %v", status, answer)
	}
	if status, answer, _ := admin.call(http.MethodPatch, "/scopes/shared/folders/"+added["sftp"], map[string]any{"passphrase": ""}); status !=
		http.StatusBadRequest || answer["error"] != "invalid_share_key" {
		t.Errorf("a key without its passphrase: %d %v", status, answer)
	}

	for _, test := range []struct {
		body map[string]any
		code string
	}{
		{map[string]any{"path": "s3://media:9000/movies", "endpoint": "http://127.0.0.1:9"}, "invalid_share_address"},
		{map[string]any{"path": "sftp://reader:secret@127.0.0.1/movies"}, "invalid_share_address"},
		{map[string]any{"path": "rclone:///movies", "rcloneRemote": "media"}, "invalid_share_address"},
		{map[string]any{"path": "sftp://127.0.0.1:70000/movies"}, "invalid_share_address"},
		{map[string]any{"path": "s3://media/movies"}, "invalid_share_endpoint"},
		{map[string]any{"path": "s3://media/movies", "endpoint": "ftp://127.0.0.1"}, "invalid_share_endpoint"},
		{map[string]any{"path": "sftp://127.0.0.1/movies", "privateKey": "not a key"}, "invalid_share_key"},
		{map[string]any{"path": "rclone://127.0.0.1:5572/movies"}, "invalid_rclone_remote"},
		{map[string]any{"path": "rclone://127.0.0.1:5572/movies", "rcloneRemote": "media:x"}, "invalid_rclone_remote"},
		{map[string]any{"path": "rclone://127.0.0.1:5572/movies", "rcloneRemote": "media", "pollMinutes": -1}, "invalid_poll_minutes"},
	} {
		test.body["name"], test.body["kind"] = "Broken", "movies"
		status, answer, _ := admin.call(http.MethodPost, "/scopes/shared/folders", test.body)
		if status != http.StatusBadRequest || answer["error"] != test.code {
			t.Errorf("%v: %d %v", test.body, status, answer)
		}
		if status, answer, _ := admin.call(http.MethodPatch, "/scopes/shared/folders/"+added["rclone"], map[string]any{"path": test.body["path"]}); test.code == "invalid_share_address" &&
			(status != http.StatusBadRequest || answer["error"] != test.code) {
			t.Errorf("a change to %v: %d %v", test.body["path"], status, answer)
		}
	}

	// The remotes of an rclone instance that does not answer.
	status, answer, _ = admin.call(http.MethodPost, "/scopes/shared/folders/rclone-remotes", map[string]any{"address": "http://127.0.0.1:9",
		"folder": added["rclone"]})
	if status != http.StatusOK || answer["error"] != "unreachable" {
		t.Errorf("remotes: %d %v", status, answer)
	}
}
