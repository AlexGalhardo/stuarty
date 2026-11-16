package timeline

import "testing"

func TestValidateGrade(t *testing.T) {
	line := []CurvePoint{{X: 0, Y: 0}, {X: 1, Y: 1}}
	cases := []invalidCase{
		{"hsl range unknown", func(d *Document) {
			editItem(d, clipID, func(i *Item) { i.Hsl = []HslBand{{Range: "pink", Hue: 0.1}} })
		}, VShape},
		{"hsl range twice", func(d *Document) {
			editItem(d, clipID, func(i *Item) { i.Hsl = []HslBand{{Range: "red"}, {Range: "red"}} })
		}, VShape},
		{"hsl saturation above 1", func(d *Document) {
			editItem(d, clipID, func(i *Item) { i.Hsl = []HslBand{{Range: "red", Saturation: 1.5}} })
		}, VShape},
		{"wheel hue above 360", func(d *Document) {
			editItem(d, clipID, func(i *Item) { i.Wheels = &Wheels{Shadows: &Wheel{Hue: 361}} })
		}, VShape},
		{"curve one point", func(d *Document) {
			editItem(d, clipID, func(i *Item) { i.Curves = &Curves{Red: line[:1]} })
		}, VShape},
		{"curve x not increasing", func(d *Document) {
			editItem(d, clipID, func(i *Item) {
				i.Curves = &Curves{Blue: []CurvePoint{{X: 0.5, Y: 0}, {X: 0.5, Y: 1}}}
			})
		}, VShape},
		{"grade on a text item", func(d *Document) {
			editItem(d, capItemID, func(i *Item) { i.Hsl = []HslBand{{Range: "red", Hue: 0.1}} })
		}, VShape},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := base(t)
			c.edit(d)
			if got := codesOf(Validate(d)); len(got) != 1 || got[0] != c.want {
				t.Errorf("codes %v, want only %s\n%v", got, c.want, Validate(d))
			}
		})
	}

	d := base(t)
	editItem(d, clipID, func(i *Item) {
		i.Hsl = []HslBand{{Range: "red", Hue: 0.1}}
		i.Wheels = &Wheels{Midtones: &Wheel{Hue: 360, Amount: 1, Luma: -1}}
		i.Curves = &Curves{Master: line}
	})
	if vs := Validate(d); len(vs) != 0 {
		t.Fatalf("a valid grade is accepted, got %v", vs)
	}
}
