package output

const (
	noColor      = "\u001b[0m"
	red          = "\u001b[41;1m"
	green        = "\u001b[42;1m"
	yellow       = "\u001b[43;1m"
	blue         = "\u001b[44;1m"
	magenta      = "\u001b[45;1m"
	cyan         = "\u001b[46;1m"
	lightGray    = "\u001b[47;1m"
	darkGray     = "\u001b[100;1m"
	lightRed     = "\u001b[101;1m"
	lightGreen   = "\u001b[102;1m"
	lightYellow  = "\u001b[103;1m"
	lightBlue    = "\u001b[104;1m"
	lightMagenta = "\u001b[105;1m"
	lightCyan    = "\u001b[106;1m"
)

type Color int

const (
	None Color = iota
	Red
	Green
	Yellow
	Blue
	Magenta
	Cyan
	LightGray
	DarkGray
	LightRed
	LightGreen
	LightYellow
	LightBlue
	LightMagenta
	LightCyan
)

func (c Color) String() string {
	return []string{
		noColor,
		red,
		green,
		yellow,
		blue,
		magenta,
		cyan,
		lightGray,
		darkGray,
		lightRed,
		lightGreen,
		lightYellow,
		lightBlue,
		lightMagenta,
		lightCyan}[c]
}
