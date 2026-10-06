package jellyfin

import (
	"net/http"
	"runtime"
	"strings"
)

func (h *Handler) publicInfo(r *http.Request) PublicSystemInfo {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return PublicSystemInfo{
		// The address this request reached, which apps can use to reach the
		// server again from the same network.
		LocalAddress:           scheme + "://" + r.Host,
		ServerName:             h.Accounts.Settings().ServerName,
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
	writeJSON(w, http.StatusOK, SystemInfo{
		WebSocketPortNumber:      h.WebSocketPort,
		CompletedInstallations:   []struct{}{},
		CastReceiverApplications: castReceivers,
		EncoderLocation:          "System",
		SystemArchitecture:       architecture(runtime.GOARCH),
		PublicSystemInfo:         h.publicInfo(r),
	})
}

// namedConfiguration answers a named part of the server configuration for
// any signed-in user, as Jellyfin does: encoding, which jellyfin-web reads
// before rendering ASS subtitles, and skips them when this fails, and
// which loads the fallback fonts when they are enabled (see
// encodingConfiguration). Branding has its own routes (see
// brandingConfiguration). Any other key is unknown.
func (h *Handler) namedConfiguration(w http.ResponseWriter, r *http.Request) {
	if strings.EqualFold(r.PathValue("key"), "encoding") {
		writeJSON(w, http.StatusOK, h.encodingConfiguration())
		return
	}
	processingError(w, http.StatusNotFound)
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
