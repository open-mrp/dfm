package constants

// StockForm is the shape raw material comes in. It is an open enum: processes to come add forms (a block for machining, yarn for knitting), so a client treats a value it does not know as one it does not handle, not as an error.
type StockForm string

const (
	// StockFormSheet is flat sheet or plate, described by its thickness.
	StockFormSheet StockForm = "sheet"
)

var stockForms = []StockForm{StockFormSheet}

func (f StockForm) IsValid() bool        { return isOneOf(f, stockForms) }
func (f StockForm) EnumValues() []string { return enumValues(stockForms) }

// Open marks the enum as gaining values: processes bring their stock forms.
func (f StockForm) Open() bool { return true }
