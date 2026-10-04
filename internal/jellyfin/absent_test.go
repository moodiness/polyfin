package jellyfin

import (
	"net/http"
	"strings"
	"testing"
)

// Dashboard features Polyfin does without answer as an unconfigured
// Jellyfin 12.1: empty lists, defaults and Jellyfin's own refusals, to
// administrators only where Jellyfin requires one.
func TestAbsentFeaturesAnswerAsAnUnconfiguredJellyfin(t *testing.T) {
	s, admin, member := administrated(t)
	const plugin = "/Plugins/00000000-0000-0000-0000-000000000001"
	for _, c := range []struct {
		method, path string
		body         any
		// status is the administrator's; fixture or want checks the body.
		status        int
		fixture, want string
		// memberStatus is a member's: 403 where Jellyfin requires an
		// administrator.
		memberStatus int
	}{
		{"GET", "/Plugins", nil, 200, "plugins", "[]", 403},
		{"POST", plugin + "/1.0.0.0/Enable", nil, 404, "", `"Not Found"`, 403},
		{"POST", plugin + "/1.0.0.0/Disable", nil, 404, "", `"Not Found"`, 403},
		{"DELETE", plugin + "/1.0.0.0", nil, 404, "", `"Not Found"`, 403},
		{"DELETE", plugin, nil, 404, "", `"Not Found"`, 403},
		{"GET", plugin + "/Configuration", nil, 404, "", `"Not Found"`, 403},
		{"POST", plugin + "/Configuration", map[string]string{}, 404, "", `"Not Found"`, 403},
		{"GET", plugin + "/1.0.0.0/Image", nil, 404, "", `"Not Found"`, 403},
		{"POST", plugin + "/Manifest", nil, 404, "", `"Not Found"`, 403},
		{"GET", "/Plugins/x/Configuration", nil, 400, "", `The value 'x' is not valid.`, 403},
		{"GET", "/Packages", nil, 200, "packages", "[]", 403},
		{"GET", "/Packages/Example", nil, 404, "", `"Not Found"`, 403},
		{"POST", "/Packages/Installed/Example", nil, 404, "", `"Not Found"`, 403},
		{"DELETE", "/Packages/Installing/00000000-0000-0000-0000-000000000001", nil, 204, "", "", 403},
		{"GET", "/Repositories", nil, 200, "repositories", "[]", 403},
		{"POST", "/Repositories", []map[string]any{{"Name": "x", "Url": "http://x", "Enabled": true}}, 403, "", "", 403},
		{"GET", "/Environment/DefaultDirectoryBrowser", nil, 200, "default-directory-browser", "{}", 403},
		{"GET", "/Environment/DirectoryContents?path=/", nil, 200, "", "[]", 403},
		{"GET", "/Environment/DirectoryContents", nil, 400, "", "The path field is required.", 403},
		{"GET", "/Environment/Drives", nil, 200, "drives", "[]", 403},
		{"GET", "/Environment/ParentPath?path=/a/b", nil, 200, "", `"/a"`, 403},
		{"GET", "/Environment/ParentPath?path=/a", nil, 200, "", `"/"`, 403},
		{"GET", "/Environment/ParentPath?path=/", nil, 200, "", "null", 403},
		{"GET", "/Environment/ParentPath?path=a", nil, 200, "", `""`, 403},
		{"POST", "/Environment/ValidatePath", map[string]any{"Path": "/"}, 404, "", `"Not Found"`, 403},
		{"GET", "/Library/PhysicalPaths", nil, 200, "physical-paths", "[]", 403},
		{"GET", "/Libraries/AvailableOptions?libraryContentType=movies", nil, 200, "available-options-movies", `"Type":"Movie"`, 200},
		{"GET", "/System/Configuration/MetadataOptions/Default", nil, 200, "default-metadata-options", "", 403},
		{"POST", "/LiveTv/TunerHosts", map[string]string{"Type": "m3u"}, 404, "", "Error processing request.", 403},
		{"DELETE", "/LiveTv/TunerHosts?id=x", nil, 204, "", "", 403},
		{"GET", "/LiveTv/TunerHosts", nil, 405, "", "", 405},
		{"GET", "/LiveTv/TunerHosts/Types", nil, 200, "tuner-host-types", "[]", 200},
		{"GET", "/LiveTv/Tuners/Discover", nil, 200, "discovered-tuners", "[]", 403},
		{"GET", "/LiveTv/Tuners/Discvover", nil, 200, "discovered-tuners", "[]", 403},
		{"POST", "/LiveTv/Tuners/x/Reset", nil, 400, "", "Error processing request.", 403},
		{"POST", "/LiveTv/ListingProviders", map[string]string{"Type": "xmltv"}, 404, "", "Error processing request.", 403},
		{"DELETE", "/LiveTv/ListingProviders?id=x", nil, 204, "", "", 403},
		{"GET", "/LiveTv/ListingProviders/Default", nil, 200, "default-listing-provider", "", 200},
		{"GET", "/LiveTv/ListingProviders/Lineups?id=x", nil, 404, "", "Error processing request.", 404},
		{"GET", "/LiveTv/ListingProviders/SchedulesDirect/Countries", nil, 500, "", "Error processing request.", 403},
		{"GET", "/LiveTv/ChannelMappingOptions?providerId=x", nil, 500, "", "Error processing request.", 403},
		{"POST", "/LiveTv/ChannelMappings", map[string]string{"ProviderId": "x", "TunerChannelId": "a", "ProviderChannelId": "b"}, 500, "", "Error processing request.", 403},
		{"GET", "/Startup/Configuration", nil, 200, "startup-configuration", `"MetadataCountryCode":"US"`, 403},
		{"GET", "/Startup/User", nil, 200, "startup-user", `{"Name":"root"}`, 403},
		{"GET", "/Startup/FirstUser", nil, 200, "startup-user", `{"Name":"root"}`, 403},
		{"POST", "/Startup/Complete", nil, 204, "", "", 403},
		{"POST", "/Startup/User", map[string]string{"Name": "x", "Password": "y"}, 403, "", "", 403},
		{"POST", "/Startup/Configuration", map[string]string{"ServerName": "x"}, 403, "", "", 403},
		{"POST", "/Startup/RemoteAccess", map[string]bool{"EnableRemoteAccess": true}, 403, "", "", 403},
		{"GET", "/Backup", nil, 200, "backups", "[]", 403},
		{"POST", "/Backup/Create", map[string]any{}, 403, "", "", 403},
		{"GET", "/Backup/Manifest?path=x", nil, 404, "", `"Not Found"`, 403},
		{"GET", "/Backup/Manifest", nil, 400, "", "path", 403},
		{"POST", "/Backup/Restore", map[string]string{"ArchiveFileName": "x.zip"}, 404, "", `"Not Found"`, 403},
	} {
		name := c.method + " " + c.path
		status, body := s.call(c.method, c.path, admin, c.body)
		if status != c.status || !strings.Contains(string(body), c.want) {
			t.Errorf("%s: %d %s", name, status, body)
		}
		if c.fixture != "" {
			matchesFixture(t, c.fixture, body, shapeRules{})
		}
		if status, body := s.call(c.method, c.path, member, c.body); status != c.memberStatus || c.memberStatus == http.StatusForbidden && len(body) != 0 {
			t.Errorf("%s for a member: %d %q", name, status, body)
		}
		if status, _ := s.call(c.method, c.path, "", c.body); status != http.StatusUnauthorized && c.status != http.StatusMethodNotAllowed {
			t.Errorf("%s anonymously: %d", name, status)
		}
	}
}

// The wizard's configuration is Polyfin's.
func TestStartupConfigurationFollowsTheSettings(t *testing.T) {
	s, admin, _ := administrated(t)
	if status, body := s.call(http.MethodPost, "/System/Configuration", admin, map[string]string{"ServerName": "Home", "UICulture": "fr"}); status != http.StatusNoContent {
		t.Fatalf("configure: %d %s", status, body)
	}
	if _, body := s.call(http.MethodGet, "/Startup/Configuration", admin, nil); !strings.Contains(string(body), `"ServerName":"Home","UICulture":"fr"`) {
		t.Errorf("startup configuration: %s", body)
	}
}
