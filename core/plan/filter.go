package plan

// filterMatrices are the colour matrices of the fixed filters of SDD-001 §3.4, 3x4,
// row-major, applied to non-linear RGB in 0..1 (SDD-004 §3.6). Each row is
// {r, g, b, offset}.
var filterMatrices = map[string][12]float64{
	"grayscale": {
		0.2126, 0.7152, 0.0722, 0,
		0.2126, 0.7152, 0.0722, 0,
		0.2126, 0.7152, 0.0722, 0,
	},
	"sepia": {
		0.393, 0.769, 0.189, 0,
		0.349, 0.686, 0.168, 0,
		0.272, 0.534, 0.131, 0,
	},
	"invert": {
		-1, 0, 0, 1,
		0, -1, 0, 1,
		0, 0, -1, 1,
	},
	"warm": {
		1.1, 0, 0, 0,
		0, 1, 0, 0,
		0, 0, 0.9, 0,
	},
	"cool": {
		0.9, 0, 0, 0,
		0, 1, 0, 0,
		0, 0, 1.1, 0,
	},
}

// FilterMatrix is the matrix of a filter mixed with the identity by amount (0 is
// the original, 1 the full filter), rounded to 3 decimals. It is nil when the
// filter is none or unknown, or the amount is 0: the layer then carries no filter.
func FilterMatrix(name string, amount float64) *[12]float64 {
	f, ok := filterMatrices[name]
	if !ok || amount <= 0 {
		return nil
	}

	amount = min(amount, 1)

	var m [12]float64

	for i := range 3 {
		for j := range 4 {
			id := 0.0
			if i == j {
				id = 1
			}

			m[i*4+j] = round3((1-amount)*id + amount*f[i*4+j])
		}
	}

	return &m
}
