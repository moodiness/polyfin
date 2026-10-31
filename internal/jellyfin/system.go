package jellyfin

import (
	"net/http"
	"runtime"
	"strings"

	"github.com/moodiness/polyfin/internal/accounts"
)

func (h *Handler) publicInfo(r *http.Request) PublicSystemInfo {
	settings := h.Accounts.Settings()
	// The address on the local network the settings give, as Jellyfin's
	// PublishedServerUrl, which discovery answers announce too; else the
	// address this request reached, which apps can use to reach the server
	// again from the same network.
	local := settings.LocalAddress
	if local == "" {
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		local = scheme + "://" + r.Host
	}
	return PublicSystemInfo{
		LocalAddress:           local,
		ServerName:             settings.ServerName,
		Version:                Version,
		ProductName:            productName,
		Id:                     h.ServerID,
		StartupWizardCompleted: true,
	}
}

func (h *Handler) publicSystemInfo(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.publicInfo(r))
}

func (h *Handler) systemInfo(w http.ResponseWriter, r *http.Request) {
	_, update := h.Updates.Available()
	writeJSON(w, http.StatusOK, SystemInfo{
		WebSocketPortNumber:      h.WebSocketPort,
		CompletedInstallations:   []struct{}{},
		CastReceiverApplications: castReceivers,
		HasUpdateAvailable:       update,
		EncoderLocation:          "System",
		SystemArchitecture:       architecture(runtime.GOARCH),
		PublicSystemInfo:         h.publicInfo(r),
	})
}

// namedConfiguration answers a named part of the server configuration for
// any signed-in user, as Jellyfin does: encoding, which jellyfin-web reads
// before rendering ASS subtitles, and skips them when this fails, and
// which loads the fallback fonts when they are enabled (see
// encodingConfiguration); livetv, which lists the tuners (see
// liveTvConfiguration); and network, which jellyfin-web's user editor
// reads before showing "Allow remote connections to this server" (see
// networkConfiguration). Branding has its own routes (see
// serverBranding). Any other key is unknown.
func (h *Handler) namedConfiguration(w http.ResponseWriter, r *http.Request) {
	switch key := r.PathValue("key"); {
	case strings.EqualFold(key, "encoding"):
		writeJSON(w, http.StatusOK, h.encodingConfiguration())
	case strings.EqualFold(key, "livetv"):
		h.liveTvConfiguration(w, r)
	case strings.EqualFold(key, "network"):
		writeJSON(w, http.StatusOK, h.networkConfiguration())
	default:
		processingError(w, http.StatusNotFound)
	}
}

// NetworkConfiguration is Jellyfin's network configuration, in its order
// (NetworkConfiguration.cs, 12.4).
type NetworkConfiguration struct {
	BaseUrl                           string
	EnableHttps                       bool
	RequireHttps                      bool
	CertificatePath                   string
	CertificatePassword               string
	InternalHttpPort                  int
	InternalHttpsPort                 int
	PublicHttpPort                    int
	PublicHttpsPort                   int
	AutoDiscovery                     bool
	EnableUPnP                        bool
	EnableIPv4                        bool
	EnableIPv6                        bool
	EnableRemoteAccess                bool
	LocalNetworkSubnets               []string
	LocalNetworkAddresses             []string
	KnownProxies                      []string
	IgnoreVirtualInterfaces           bool
	VirtualInterfaceNames             []string
	EnablePublishedServerUriByRequest bool
	PublishedServerUriBySubnet        []string
	RemoteIPFilter                    []string
	IsRemoteIPFilterBlacklist         bool
}

// networkConfiguration is Jellyfin's defaults with the server's port,
// local discovery and Known proxies. Polyfin answers outside the local
// network, so EnableRemoteAccess is true, which has jellyfin-web's user
// editor show each user's own remote access. The local network is
// Jellyfin's default, the private ranges, so no subnet is listed.
func (h *Handler) networkConfiguration() NetworkConfiguration {
	settings := h.Accounts.Settings()
	return NetworkConfiguration{
		InternalHttpPort: h.WebSocketPort, InternalHttpsPort: 8920, PublicHttpPort: h.WebSocketPort, PublicHttpsPort: 8920,
		AutoDiscovery: settings.LocalDiscovery, EnableIPv4: true, EnableRemoteAccess: true,
		LocalNetworkSubnets: []string{}, LocalNetworkAddresses: []string{},
		KnownProxies:            accounts.FormatKnownProxies(settings.KnownProxies, settings.KnownProxyNames),
		IgnoreVirtualInterfaces: true, VirtualInterfaceNames: []string{"veth"},
		PublishedServerUriBySubnet: []string{}, RemoteIPFilter: []string{},
	}
}

// encodingConfiguration is the encoding configuration Polyfin answers:
// Jellyfin's defaults, with the fallback fonts enabled when the fonts
// folder has some.
func (h *Handler) encodingConfiguration() EncodingOptions {
	options := encodingOptions
	options.EnableFallbackFont = len(h.fallbackFontFiles()) > 0
	return options
}

// EncodingOptions is Jellyfin's encoding configuration, in its order.
type EncodingOptions struct {
	EncodingThreadCount                                       int
	EnableFallbackFont                                        bool
	EnableAudioVbr                                            bool
	DownMixAudioBoost                                         float64
	DownMixStereoAlgorithm                                    string
	MaxMuxingQueueSize                                        int
	EnableThrottling                                          bool
	ThrottleDelaySeconds                                      int
	EnableSegmentDeletion                                     bool
	SegmentKeepSeconds                                        int
	HardwareAccelerationType                                  string
	VaapiDevice                                               string
	QsvDevice                                                 string
	EnableTonemapping                                         bool
	EnableVppTonemapping                                      bool
	EnableVideoToolboxTonemapping                             bool
	TonemappingAlgorithm                                      string
	TonemappingMode                                           string
	TonemappingRange                                          string
	TonemappingDesat                                          float64
	TonemappingPeak                                           float64
	TonemappingParam                                          float64
	VppTonemappingBrightness                                  float64
	VppTonemappingContrast                                    float64
	H264Crf                                                   int
	H265Crf                                                   int
	EncoderPreset                                             string
	DeinterlaceDoubleRate                                     bool
	DeinterlaceMethod                                         string
	EnableDecodingColorDepth10Hevc                            bool
	EnableDecodingColorDepth10Vp9                             bool
	EnableDecodingColorDepth10HevcRext                        bool
	EnableDecodingColorDepth12HevcRext                        bool
	EnableEnhancedNvdecDecoder                                bool
	PreferSystemNativeHwDecoder                               bool
	EnableIntelLowPowerH264HwEncoder                          bool
	EnableIntelLowPowerHevcHwEncoder                          bool
	EnableHardwareEncoding                                    bool
	AllowHevcEncoding                                         bool
	AllowAv1Encoding                                          bool
	EnableSubtitleExtraction                                  bool
	SubtitleExtractionTimeoutMinutes                          int
	HardwareDecodingCodecs                                    []string
	AllowOnDemandMetadataBasedKeyframeExtractionForExtensions []string
	HlsAudioSeekStrategy                                      string
}

// encodingOptions are Jellyfin 12.2's defaults, as a new server answers
// them; encodingConfiguration enables the fallback fonts when there are
// some.
// Polyfin's own encoding settings, under the admin app's Conversion
// settings, are not reported here: Polyfin's defaults differ from
// Jellyfin's, and jellyfin-web's dashboard pages open the admin app. The
// path of Jellyfin's FFmpeg is left out.
var encodingOptions = EncodingOptions{
	EncodingThreadCount: -1, DownMixAudioBoost: 2, DownMixStereoAlgorithm: "None", MaxMuxingQueueSize: 2048,
	ThrottleDelaySeconds: 180, SegmentKeepSeconds: 720, HardwareAccelerationType: "none", VaapiDevice: "/dev/dri/renderD128",
	TonemappingAlgorithm: "bt2390", TonemappingMode: "auto", TonemappingRange: "auto", TonemappingPeak: 100,
	VppTonemappingBrightness: 16, VppTonemappingContrast: 1, H264Crf: 23, H265Crf: 28, EncoderPreset: "auto",
	DeinterlaceMethod: "yadif", EnableDecodingColorDepth10Hevc: true, EnableDecodingColorDepth10Vp9: true,
	EnableEnhancedNvdecDecoder: true, PreferSystemNativeHwDecoder: true, EnableHardwareEncoding: true,
	EnableSubtitleExtraction: true, SubtitleExtractionTimeoutMinutes: 30, HardwareDecodingCodecs: []string{"h264", "vc1"},
	AllowOnDemandMetadataBasedKeyframeExtractionForExtensions: []string{"mkv"}, HlsAudioSeekStrategy: "TrimCopiedAudio",
}

// architecture names GOARCH values as .NET does.
func architecture(goarch string) string {
	switch goarch {
	case "amd64":
		return "X64"
	case "arm64":
		return "Arm64"
	case "386":
		return "X86"
	case "arm":
		return "Arm"
	default:
		return goarch
	}
}

func (h *Handler) ping(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, productName)
}
