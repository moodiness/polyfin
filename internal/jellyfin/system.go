package jellyfin

import (
	"net/http"
	"runtime"
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
