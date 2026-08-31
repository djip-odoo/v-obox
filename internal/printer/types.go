package printer

// Type is what a printer produces, which decides how job data is framed.
type Type string

const (
	TypeReceipt Type = "receipt"
	TypeLabel   Type = "label"
)
