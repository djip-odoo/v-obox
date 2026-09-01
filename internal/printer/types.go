package printer

// Type is what a printer produces, which decides how job data is framed.
type Type string

const (
	TypeReceipt Type = "receipt"
	TypeLabel   Type = "label"
)

// LibUsbPrinter is the identity read off a device during a scan.
type LibUsbPrinter struct {
	Serial string
	Path   string
	Name   string
	VidPid string
}
