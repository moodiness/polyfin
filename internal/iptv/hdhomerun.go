package iptv

import (
	"bytes"
	"cmp"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// An HDHomeRun tuner is an IPTV source of the local network: its address is
// the tuner's own (http://192.168.1.20), its channels are those of its
// line-up (lineup.json) but for the protected ones, and its tuner count is
// how many channels it plays at once. Only administrators add tuners, as
// only their sources may reach the local network.

// TunerPort is the UDP port HDHomeRun tuners answer discovery on.
const TunerPort = 65001

const (
	// tunerAnswer bounds each request to a tuner; maxTunerJSON its
	// discover.json, lineup.json and lineup_status.json.
	tunerAnswer  = 15 * time.Second
	maxTunerJSON = 8 << 20
	// tunerDiscovery is how long Find tuners waits for the tuners of the
	// local network to answer its broadcast; tunerDiscoveryMost how long
	// it waits for addresses it asked one by one, which all answer.
	tunerDiscovery     = 3 * time.Second
	tunerDiscoveryMost = 15 * time.Second
	// scanPoll is how often a scan's progress is read, and scanMost how
	// long a scan is followed at most.
	scanPoll = 2 * time.Second
	scanMost = 30 * time.Minute
)

// ErrTunersRecording reports a recording that found every tuner of its
// channel's tuner recording another channel.
var ErrTunersRecording = errors.New("every tuner is recording another channel")

// ErrNotTuner reports a tuner action on a source that is not a tuner.
var ErrNotTuner = errors.New("the source is not a tuner")

// Tuner is what an HDHomeRun tuner says of itself: its device identifier,
// name, model, how many channels it plays at once, and its address.
type Tuner struct {
	DeviceID     string
	FriendlyName string
	ModelNumber  string
	TunerCount   int
	BaseURL      string
}

// tunerAddress checks a tuner's address, "192.168.1.20" or "http://…", and
// returns the address it is stored under: its scheme and host.
func tunerAddress(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > maxAddress {
		return "", ErrInvalidAddress
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "http" && parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil ||
		parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", ErrInvalidAddress
	}
	switch strings.TrimSuffix(parsed.Path, "/") {
	case "", "/discover.json", "/lineup.json":
	default:
		return "", ErrInvalidAddress
	}
	return parsed.Scheme + "://" + parsed.Host, nil
}

// flag reads a lineup.json flag, which tuners write 1, and some true.
type flag bool

func (f *flag) UnmarshalJSON(data []byte) error {
	switch strings.Trim(string(data), `"`) {
	case "1", "true":
		*f = true
	default:
		*f = false
	}
	return nil
}

type discoverJSON struct {
	FriendlyName string
	ModelNumber  string
	DeviceID     string
	TunerCount   int
	BaseURL      string
}

type lineupJSON struct {
	GuideNumber string
	GuideName   string
	URL         string
	DRM         flag
	Favorite    flag
}

// tunerGet reads a tuner's JSON document at address into into.
func tunerGet(ctx context.Context, client *stremio.Client, address string, confined bool, into any) error {
	ctx, cancel := context.WithTimeout(ctx, tunerAnswer)
	defer cancel()
	response, err := client.Open(ctx, http.MethodGet, address, nil, confined)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: HTTP %d", stremio.ErrUnreachable, response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxTunerJSON+1))
	if err != nil {
		return fmt.Errorf("%w: %v", stremio.ErrUnreachable, err)
	}
	if len(data) > maxTunerJSON {
		return ErrTooLarge
	}
	if err := json.Unmarshal(data, into); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidList, err)
	}
	return nil
}

// describeTuner reads a tuner's discover.json.
func describeTuner(ctx context.Context, client *stremio.Client, base string, confined bool) (Tuner, error) {
	var d discoverJSON
	if err := tunerGet(ctx, client, base+"/discover.json", confined, &d); err != nil {
		return Tuner{}, err
	}
	if d.DeviceID == "" && d.ModelNumber == "" && d.TunerCount <= 0 {
		return Tuner{}, ErrInvalidList
	}
	return Tuner{DeviceID: strings.ToUpper(strings.TrimSpace(d.DeviceID)), FriendlyName: strings.TrimSpace(d.FriendlyName),
		ModelNumber: strings.TrimSpace(d.ModelNumber), TunerCount: max(d.TunerCount, 0), BaseURL: base}, nil
}

// fetchTuner reads a tuner's description and line-up: its channels with
// their number and name, but for those it marks protected (DRM), which
// it does not play.
func fetchTuner(ctx context.Context, client requester, base string, confined bool) (downloaded, error) {
	tuner, err := describeTuner(ctx, client.client, base, confined)
	if err != nil {
		return downloaded{}, err
	}
	var lineup []lineupJSON
	if err := tunerGet(ctx, client.client, base+"/lineup.json", confined, &lineup); err != nil {
		return downloaded{}, err
	}
	d := downloaded{got: livePart, tuner: &tuner}
	if tuner.TunerCount > 0 {
		d.connections = &tuner.TunerCount
	}
	seen := map[string]bool{}
	for _, c := range lineup {
		number := strings.TrimSpace(c.GuideNumber)
		if bool(c.DRM) || number == "" || seen[number] || !webAddress(c.URL) {
			continue
		}
		if len(d.entries) >= maxChannels {
			return downloaded{}, ErrTooLarge
		}
		seen[number] = true
		major, _, _ := strings.Cut(number, ".")
		n, _ := strconv.Atoi(major)
		d.entries = append(d.entries, Entry{ID: number, Name: cmp.Or(strings.TrimSpace(c.GuideName), number), Number: max(n, 0),
			GuideNumber: number, URL: c.URL, Favorite: bool(c.Favorite)})
	}
	return d, nil
}

// storeTuner stores what a tuner said of itself: its tuner count is its
// connection limit.
func storeTuner(ctx context.Context, tx pgx.Tx, source accounts.ID, tuner Tuner) error {
	var count *int
	if tuner.TunerCount > 0 {
		count = &tuner.TunerCount
	}
	_, err := tx.Exec(ctx, "UPDATE iptv_sources SET tuner_device = $2, tuner_model = $3, max_connections = $4 WHERE addon_id = $1",
		source, tuner.DeviceID, tuner.ModelNumber, count)
	return err
}

// Limit is how many streams of a source play at once: Connections, 0 for
// no limit; Tuners tells a tuner, whose recordings take the tuners of
// channels only watched.
type Limit struct {
	Connections int
	Tuners      bool
}

// TunerState is what the admin app shows of a tuner: its device and model,
// the last failure of its streams other than its tuners being in use, and
// the scan under way, if any.
type TunerState struct {
	Device   string
	Model    string
	Failure  string
	FailedAt *time.Time
	Scan     *ScanState
}

// ScanState is a tuner's channel scan under way: its progress in percent
// and the channels found so far.
type ScanState struct {
	Progress int
	Found    int
}

// scans are the tuners' channel scans under way, by source.
type scans struct {
	mu   sync.Mutex
	runs map[accounts.ID]*scanRun
}

type scanRun struct {
	state ScanState
	done  chan struct{}
}

// scanState returns a source's scan under way, nil for none.
func (s *Service) scanState(source accounts.ID) *ScanState {
	s.scans.mu.Lock()
	defer s.scans.mu.Unlock()
	if run, ok := s.scans.runs[source]; ok {
		state := run.state
		return &state
	}
	return nil
}

type statusJSON struct {
	ScanInProgress int
	Progress       int
	Found          int
	Source         string
	SourceList     []string
}

// Scan asks a tuner of the scope to scan for channels again, then reads
// its line-up once the scan ended. It returns once the tuner started
// scanning; done is closed once the line-up was read again, or the scan
// given up. A scan under way is not started again.
func (s *Service) Scan(ctx context.Context, scope addons.Scope, id accounts.ID) (done <-chan struct{}, err error) {
	current, err := s.source(ctx, scope, id)
	if err != nil {
		return nil, err
	}
	addon := current.Addon
	if !addon.Tuner() {
		return nil, ErrNotTuner
	}
	s.scans.mu.Lock()
	if s.scans.runs == nil {
		s.scans.runs = map[accounts.ID]*scanRun{}
	}
	if run, ok := s.scans.runs[id]; ok {
		s.scans.mu.Unlock()
		return run.done, nil
	}
	run := &scanRun{done: make(chan struct{})}
	s.scans.runs[id] = run
	s.scans.mu.Unlock()
	finish := func() {
		s.scans.mu.Lock()
		delete(s.scans.runs, id)
		s.scans.mu.Unlock()
		close(run.done)
	}
	base := addon.ManifestURL
	var status statusJSON
	err = tunerGet(ctx, s.client, base+"/lineup_status.json", false, &status)
	if err == nil {
		source := cmp.Or(status.Source, "Antenna")
		if len(status.SourceList) > 0 && !slices.Contains(status.SourceList, source) {
			source = status.SourceList[0]
		}
		err = s.startScan(ctx, base, source)
	}
	if err != nil {
		finish()
		return nil, err
	}
	s.logger.Info("A tuner scans for channels", "source", addon.Manifest.Name)
	go func() {
		defer finish()
		ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), scanMost)
		defer cancel()
		poll := cmp.Or(s.scanPoll, scanPoll)
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(poll):
			}
			var status statusJSON
			if err := tunerGet(ctx, s.client, base+"/lineup_status.json", false, &status); err != nil {
				s.logger.Warn("A tuner's scan could not be followed", "source", addon.Manifest.Name, "error", err)
				break
			}
			if status.ScanInProgress == 0 {
				break
			}
			s.scans.mu.Lock()
			run.state = ScanState{Progress: min(max(status.Progress, 0), 100), Found: max(status.Found, 0)}
			s.scans.mu.Unlock()
		}
		if err := s.refresh(ctx, addon, false); err != nil {
			s.logger.Warn("A tuner's line-up could not be read after its scan", "source", addon.Manifest.Name, "error", err)
		}
	}()
	return run.done, nil
}

// startScan asks a tuner to scan its source ("Antenna", "Cable").
func (s *Service) startScan(ctx context.Context, base, source string) error {
	ctx, cancel := context.WithTimeout(ctx, tunerAnswer)
	defer cancel()
	response, err := s.client.Open(ctx, http.MethodPost, base+"/lineup.post?"+url.Values{"scan": {"start"}, "source": {source}}.Encode(), nil, false)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
	if response.StatusCode >= 300 {
		return fmt.Errorf("%w: HTTP %d", stremio.ErrUnreachable, response.StatusCode)
	}
	return nil
}

// FoundTuner is a tuner Find tuners found, with the source it is already,
// if any.
type FoundTuner struct {
	Tuner
	Source *accounts.ID
}

// UseTunerDiscovery sets the UDP port tuners answer discovery on, and the
// addresses (host:port) discovery asks; none asks the local networks by
// broadcast. It is called before the service is used.
func (s *Service) UseTunerDiscovery(port int, addresses ...string) {
	s.tunerPort, s.tunerAddresses = port, addresses
}

// discoveryTargets are where discovery asks, and whether they are
// broadcasts: the addresses UseTunerDiscovery set, else the broadcast
// address of every local network and the limited broadcast.
func (s *Service) discoveryTargets() ([]*net.UDPAddr, bool) {
	var targets []*net.UDPAddr
	for _, address := range s.tunerAddresses {
		if target, err := net.ResolveUDPAddr("udp4", address); err == nil {
			targets = append(targets, target)
		}
	}
	if len(s.tunerAddresses) > 0 {
		return targets, false
	}
	port := cmp.Or(s.tunerPort, TunerPort)
	targets = append(targets, &net.UDPAddr{IP: net.IPv4bcast, Port: port})
	interfaces, _ := net.Interfaces()
	for _, i := range interfaces {
		if i.Flags&net.FlagUp == 0 || i.Flags&net.FlagBroadcast == 0 || i.Flags&net.FlagLoopback != 0 {
			continue
		}
		addresses, _ := i.Addrs()
		for _, a := range addresses {
			network, ok := a.(*net.IPNet)
			if !ok || network.IP.To4() == nil {
				continue
			}
			ip, mask := network.IP.To4(), net.IP(network.Mask).To4()
			if mask == nil {
				continue
			}
			broadcast := make(net.IP, 4)
			for n := range broadcast {
				broadcast[n] = ip[n] | ^mask[n]
			}
			targets = append(targets, &net.UDPAddr{IP: broadcast, Port: port})
		}
	}
	return targets, true
}

// FindTuners asks the tuners of the local network to answer (HDHomeRun's
// discovery), then reads each one's discover.json for its name and model.
// It lists them by name, with the source each already is.
func (s *Service) FindTuners(ctx context.Context) ([]FoundTuner, error) {
	targets, broadcast := s.discoveryTargets()
	answers, err := discoverTuners(ctx, targets, broadcast)
	if err != nil {
		return nil, err
	}
	found := make([]FoundTuner, len(answers))
	var wg sync.WaitGroup
	for i, answer := range answers {
		wg.Go(func() {
			found[i].Tuner = answer
			if described, err := describeTuner(ctx, s.client, answer.BaseURL, false); err == nil {
				found[i].Tuner = Tuner{DeviceID: cmp.Or(described.DeviceID, answer.DeviceID), FriendlyName: described.FriendlyName,
					ModelNumber: described.ModelNumber, TunerCount: cmp.Or(described.TunerCount, answer.TunerCount), BaseURL: answer.BaseURL}
			}
		})
	}
	wg.Wait()
	sources, err := s.tunerSources(ctx)
	if err != nil {
		return nil, err
	}
	for i := range found {
		for _, source := range sources {
			if source.device != "" && source.device == found[i].DeviceID || source.address == found[i].BaseURL {
				found[i].Source = &source.id
				break
			}
		}
	}
	slices.SortFunc(found, func(a, b FoundTuner) int {
		return cmp.Or(strings.Compare(a.FriendlyName, b.FriendlyName), strings.Compare(a.BaseURL, b.BaseURL))
	})
	return found, nil
}

type tunerSource struct {
	id              accounts.ID
	device, address string
}

func (s *Service) tunerSources(ctx context.Context) ([]tunerSource, error) {
	rows, err := s.db.Query(ctx, `SELECT a.id, i.tuner_device, a.manifest_url FROM iptv_sources i JOIN addons a ON a.id = i.addon_id
		WHERE a.kind = $1 ORDER BY a.id`, addons.KindHDHomeRun)
	if err != nil {
		return nil, err
	}
	var t tunerSource
	var list []tunerSource
	_, err = pgx.ForEachRow(rows, []any{&t.id, &t.device, &t.address}, func() error { list = append(list, t); return nil })
	return list, err
}

// Tuners describes the server's tuners, in the order of its addons.
func (s *Service) Tuners(ctx context.Context) ([]Source, error) {
	list, err := s.addons.Addons(ctx, addons.Shared())
	if err != nil {
		return nil, err
	}
	var tuners []Source
	for _, addon := range list {
		if !addon.Tuner() {
			continue
		}
		source, err := s.source(ctx, addons.Shared(), addon.ID)
		if errors.Is(err, addons.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		tuners = append(tuners, source)
	}
	return tuners, nil
}

// RemoveTuner removes a tuner of the server, and returns it: another
// source is ErrNotTuner, and leaves it be.
func (s *Service) RemoveTuner(ctx context.Context, id accounts.ID) (addons.Addon, error) {
	list, err := s.addons.Addons(ctx, addons.Shared())
	if err != nil {
		return addons.Addon{}, err
	}
	index := slices.IndexFunc(list, func(a addons.Addon) bool { return a.ID == id })
	if index < 0 {
		return addons.Addon{}, addons.ErrNotFound
	}
	if !list[index].Tuner() {
		return addons.Addon{}, ErrNotTuner
	}
	if err := s.addons.Remove(ctx, addons.Shared(), id); err != nil {
		return addons.Addon{}, err
	}
	s.forget(id)
	return list[index], nil
}

// HDHomeRun's discovery: a packet is its type and payload's length (big
// endian), its payload of tags, and the CRC-32 of both (little endian). A
// tag is its code, its value's length (one byte below 128, else two) and
// its value.
const (
	discoverRequest = 0x0002
	discoverReply   = 0x0003

	tagDeviceType = 0x01
	tagDeviceID   = 0x02
	tagTunerCount = 0x10
	tagBaseURL    = 0x2a

	deviceTypeTuner = 0x00000001
	wildcard        = 0xffffffff
)

// discoveryPacket seals a packet of type kind and payload.
func discoveryPacket(kind uint16, payload []byte) []byte {
	packet := binary.BigEndian.AppendUint16(nil, kind)
	packet = binary.BigEndian.AppendUint16(packet, uint16(len(payload)))
	packet = append(packet, payload...)
	return binary.LittleEndian.AppendUint32(packet, crc32.ChecksumIEEE(packet))
}

// appendTag adds a tag to a payload.
func appendTag(payload []byte, tag byte, value []byte) []byte {
	payload = append(payload, tag)
	if len(value) < 128 {
		payload = append(payload, byte(len(value)))
	} else {
		payload = append(payload, byte(len(value)&0x7f|0x80), byte(len(value)>>7))
	}
	return append(payload, value...)
}

// DiscoveryRequest is the packet that asks every tuner to answer.
func DiscoveryRequest() []byte {
	payload := appendTag(nil, tagDeviceType, binary.BigEndian.AppendUint32(nil, deviceTypeTuner))
	payload = appendTag(payload, tagDeviceID, binary.BigEndian.AppendUint32(nil, wildcard))
	return discoveryPacket(discoverRequest, payload)
}

// DiscoveryReply is a tuner's answer: its device identifier (hexadecimal),
// tuner count and address.
func DiscoveryReply(deviceID uint32, tuners int, baseURL string) []byte {
	payload := appendTag(nil, tagDeviceType, binary.BigEndian.AppendUint32(nil, deviceTypeTuner))
	payload = appendTag(payload, tagDeviceID, binary.BigEndian.AppendUint32(nil, deviceID))
	payload = appendTag(payload, tagTunerCount, []byte{byte(tuners)})
	payload = appendTag(payload, tagBaseURL, []byte(baseURL))
	return discoveryPacket(discoverReply, payload)
}

// readTags reads a packet of type kind: its tags by code. It reports false
// for a packet of another type, or damaged.
func readTags(packet []byte, kind uint16) (map[byte][]byte, bool) {
	if len(packet) < 8 || binary.BigEndian.Uint16(packet) != kind {
		return nil, false
	}
	length := int(binary.BigEndian.Uint16(packet[2:]))
	if len(packet) != 4+length+4 || binary.LittleEndian.Uint32(packet[4+length:]) != crc32.ChecksumIEEE(packet[:4+length]) {
		return nil, false
	}
	tags := map[byte][]byte{}
	payload := packet[4 : 4+length]
	for len(payload) >= 2 {
		tag, size := payload[0], int(payload[1])
		payload = payload[2:]
		if size&0x80 != 0 {
			if len(payload) < 1 {
				return nil, false
			}
			size = size&0x7f | int(payload[0])<<7
			payload = payload[1:]
		}
		if size > len(payload) {
			return nil, false
		}
		tags[tag], payload = payload[:size], payload[size:]
	}
	return tags, true
}

// IsDiscoveryRequest reports whether a packet asks tuners to answer.
func IsDiscoveryRequest(packet []byte) bool {
	_, ok := readTags(packet, discoverRequest)
	return ok
}

// discoverTuners asks targets for tuners and collects the answers, by
// device: for the broadcast's time, or until every target asked one by one
// answered.
func discoverTuners(ctx context.Context, targets []*net.UDPAddr, broadcast bool) ([]Tuner, error) {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{})
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	request := DiscoveryRequest()
	sent := 0
	for _, target := range targets {
		if _, err := conn.WriteToUDP(request, target); err == nil {
			sent++
		}
	}
	if sent == 0 {
		return nil, nil
	}
	wait := tunerDiscovery
	if !broadcast {
		wait = tunerDiscoveryMost
	}
	deadline := time.Now().Add(wait)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	_ = conn.SetReadDeadline(deadline)
	stop := context.AfterFunc(ctx, func() { _ = conn.SetReadDeadline(time.Now()) })
	defer stop()
	var tuners []Tuner
	answered := map[string]bool{}
	buffer := make([]byte, 1500)
	for broadcast || len(answered) < sent {
		n, from, err := conn.ReadFromUDP(buffer)
		if err != nil {
			break
		}
		tags, ok := readTags(buffer[:n], discoverReply)
		if !ok || len(tags[tagDeviceType]) != 4 || binary.BigEndian.Uint32(tags[tagDeviceType]) != deviceTypeTuner {
			continue
		}
		answered[from.String()] = true
		tuner := Tuner{BaseURL: "http://" + from.IP.String()}
		if id := tags[tagDeviceID]; len(id) == 4 {
			tuner.DeviceID = fmt.Sprintf("%08X", binary.BigEndian.Uint32(id))
		}
		if count := tags[tagTunerCount]; len(count) == 1 {
			tuner.TunerCount = int(count[0])
		}
		if base, err := tunerAddress(string(bytes.TrimRight(tags[tagBaseURL], "\x00"))); err == nil {
			tuner.BaseURL = base
		}
		if !slices.ContainsFunc(tuners, func(t Tuner) bool {
			return t.DeviceID != "" && t.DeviceID == tuner.DeviceID || t.BaseURL == tuner.BaseURL
		}) {
			tuners = append(tuners, tuner)
		}
	}
	return tuners, nil
}

// TunerFailureBusy is a stream a tuner refused as its tuners were all in
// use, which is no fault of the tuner's (see ReportStream).
const TunerFailureBusy = "busy"

// reportTuner records how a tuner's stream answered: a failure other than
// its tuners being in use shows under System › Health until a stream plays.
func (s *Service) reportTuner(ctx context.Context, source accounts.ID, failure string) error {
	if failure == TunerFailureBusy {
		return nil
	}
	var at *time.Time
	if failure != "" {
		now := s.now()
		at = &now
	}
	_, err := s.db.Exec(ctx, `UPDATE iptv_sources i SET tuner_failure = $2, tuner_failed_at = $3 FROM addons a
		WHERE i.addon_id = $1 AND a.id = i.addon_id AND a.kind = $4 AND (i.tuner_failure <> $2 OR $2 <> '')`, source, failure, at, addons.KindHDHomeRun)
	return err
}

// SourceOfChannel finds the source of a channel by its Stremio identifier,
// when it is one of an IPTV source's.
func SourceOfChannel(stremioID string) (accounts.ID, bool) {
	rest, ok := strings.CutPrefix(stremioID, IDPrefix)
	if !ok {
		return accounts.ID{}, false
	}
	raw, _, ok := strings.Cut(rest, ":")
	if !ok {
		return accounts.ID{}, false
	}
	id, err := accounts.ParseID(raw)
	return id, err == nil
}
