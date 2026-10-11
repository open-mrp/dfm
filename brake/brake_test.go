package brake

import (
	"math"
	"testing"

	"github.com/open-mrp/dfm/geometry"
)

// rect is a w x h plate, as a part.
func rect(w, h float64) *geometry.Part {
	pts := []geometry.Point{{X: 0, Y: 0}, {X: w, Y: 0}, {X: w, Y: h}, {X: 0, Y: h}}
	var c geometry.Contour
	for i, p := range pts {
		c.Segments = append(c.Segments, geometry.Segment{Start: p, End: pts[(i+1)%len(pts)], EntityID: "outline"})
	}
	c.Closed = true
	return geometry.Build([]geometry.Contour{c}, nil, geometry.BuildOptions{})
}

// channel bends a 100 mm long plate up along y = y0 and y = y1, 90° each.
func channel(t *testing.T, h, y0, y1 float64) *Model {
	t.Helper()
	bends := []Bend{
		{A: geometry.Point{X: 0, Y: y0}, B: geometry.Point{X: 100, Y: y0}, Angle: 90, InsideRadius: 1.5, Up: true},
		{A: geometry.Point{X: 0, Y: y1}, B: geometry.Point{X: 100, Y: y1}, Angle: 90, InsideRadius: 1.5, Up: true},
	}
	m, err := New(rect(100, h), bends, 1.5, 0.44, 1)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// straight is a straight punch 12 mm wide and 100 mm tall over a 12 mm die.
var straight = Tooling{PunchTipAngle: 86, PunchWidth: 12, PunchHeight: 100, PunchLength: 500, DieOpening: 12, DieWidth: 15, DieHeight: 60, DieLength: 500}

func TestFolded(t *testing.T) {
	// A 60 mm plate bent up 90° at y = 40: the 20 mm flange stands up, its top about 20 mm above the plate (less half the allowance, plus the radius and half the thickness).
	m, err := New(rect(100, 60), []Bend{{A: geometry.Point{X: 0, Y: 40}, B: geometry.Point{X: 100, Y: 40}, Angle: 90, InsideRadius: 1.5, Up: true}}, 1.5, 0.44, 1)
	if err != nil {
		t.Fatal(err)
	}
	top := math.Inf(-1)
	for _, p := range m.Folded(geometry.Point{X: 50, Y: 20}) {
		top = max(top, p[2])
	}
	rn := 1.5 + 0.44*1.5
	want := 20 - rn*math.Pi/4 + 1.5 + 0.75 // flat beyond the arc, then the arc's rise
	if math.Abs(top-want) > 1 {
		t.Errorf("flange top at %.2f mm, want about %.2f", top, want)
	}
}

// along is a 90° bend up across a 100 mm wide plate at y.
func along(y float64) Bend {
	return Bend{A: geometry.Point{X: 0, Y: y}, B: geometry.Point{X: 100, Y: y}, Angle: 90, InsideRadius: 1.5, Up: true}
}

// lipped bends a 100 mm long plate h wide into a channel with return lips: walls up at wall and h - wall, lips folded in over the base at lip and h - lip (the lips are lip mm long, the walls wall - lip tall). Every bend turns the same way, which is what folds a lip back over the base.
func lipped(t *testing.T, h, wall, lip float64) *Model {
	t.Helper()
	m, err := New(rect(100, h), []Bend{along(wall), along(h - wall), along(lip), along(h - lip)}, 1.5, 0.44, 1)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestCollisions(t *testing.T) {
	// A channel whose base is 30 mm wide with 20 mm lips folded in from both walls: the lips lie over each other.
	c := lipped(t, 90, 30, 20).Collisions()
	if len(c) != 1 || len(c[0].Bends) != 2 || c[0].Bends[0]/2 != 1 || c[0].Bends[1]/2 != 1 {
		t.Errorf("overlapping lips: %+v, want the two lip bends", c)
	}
	// The same with a 40 mm base and 10 mm lips: 20 mm apart.
	if c := lipped(t, 80, 20, 10).Collisions(); len(c) != 0 {
		t.Errorf("lips 20 mm apart: %+v, want none", c)
	}
	// A tray with walls on two adjacent edges, each set back from the other's bend line by the radius plus the thickness, clears; walls drawn to the base's corners (the arc pulls each wall 0.2 mm inside its bend line, into the other's edge) touch, which the sampling cannot see; walls that cross the corner collide.
	for _, tt := range []struct {
		name    string
		setback float64
		want    int
	}{{"set back", 3, 0}, {"full width", 0, 0}, {"past the corner", -5, 1}} {
		s := tt.setback
		pts := []geometry.Point{{X: 20, Y: 0}, {X: 70 - s, Y: 0}, {X: 70 - s, Y: 17}, {X: 67, Y: 17}, {X: 67, Y: 23}, {X: 73, Y: 23}, {X: 73, Y: 20 + s}, {X: 90, Y: 20 + s}, {X: 90, Y: 70}, {X: 20, Y: 70}}
		var k geometry.Contour
		for i, p := range pts {
			k.Segments = append(k.Segments, geometry.Segment{Start: p, End: pts[(i+1)%len(pts)], EntityID: "outline"})
		}
		k.Closed = true
		part := geometry.Build([]geometry.Contour{k}, nil, geometry.BuildOptions{})
		bends := []Bend{
			{A: geometry.Point{X: 20, Y: 20}, B: geometry.Point{X: 67, Y: 20}, Angle: 90, InsideRadius: 1.5, Up: true},
			{A: geometry.Point{X: 70, Y: 23}, B: geometry.Point{X: 70, Y: 70}, Angle: 90, InsideRadius: 1.5, Up: true},
		}
		m, err := New(part, bends, 1.5, 0.44, 1)
		if err != nil {
			t.Fatal(err)
		}
		if c := m.Collisions(); len(c) != tt.want {
			t.Errorf("tray walls %s: %+v, want %d collisions", tt.name, c, tt.want)
		}
	}
}

// TestReturnHitsBase folds a wall up, a lip in over the base and a return down from the lip: a return deeper than the wall is tall passes through the base.
func TestReturnHitsBase(t *testing.T) {
	for _, tt := range []struct {
		name  string
		ret   float64
		want  int
		below bool // the return's end reaches below the base
	}{{"short return", 8, 0, false}, {"deep return", 25, 1, true}} {
		// A 40 mm base at the top of the plate, a 15 mm wall, a 15 mm lip, then the return at the bottom edge.
		h := 70 + tt.ret
		m, err := New(rect(100, h), []Bend{along(h - 40), along(h - 55), along(h - 70)}, 1.5, 0.44, 1)
		if err != nil {
			t.Fatal(err)
		}
		bottom := math.Inf(1)
		for _, p := range m.Folded(geometry.Point{X: 50, Y: h - 20}) {
			bottom = min(bottom, p[2])
		}
		if (bottom < -1.5) != tt.below {
			t.Errorf("%s: lowest sample at %.2f mm with the base at 0", tt.name, bottom)
		}
		if c := m.Collisions(); len(c) != tt.want {
			t.Errorf("%s: %+v, want %d collisions", tt.name, c, tt.want)
		}
	}
}

func TestSequence(t *testing.T) {
	// A channel 20 mm wide with 20 mm sides swings its first side into the straight punch whichever side goes first.
	if p := channel(t, 60, 20, 40).Sequence(straight, 100); p.Found || !p.Exhausted || len(p.Blocked) == 0 {
		t.Errorf("narrow deep channel: %+v, want no order", p)
	}
	// One 40 mm wide with 10 mm sides clears it, the punch centered on each bend.
	p := channel(t, 60, 10, 50).Sequence(straight, 100)
	if !p.Found || len(p.Order) != 2 || len(p.Steps) != 2 {
		t.Fatalf("wide shallow channel: %+v, want an order", p)
	}
	for i, st := range p.Steps {
		if st.Collides() || !st.Simulated || st.Mirrored || st.Place != PlaceMiddle {
			t.Errorf("wide shallow channel step %d: %+v, want the straight punch at the middle", i, st)
		}
	}
	// With a punch only 20 mm tall, a channel 30 mm wide with 60 mm sides swings its first side over the punch, into the holder and ram above it.
	short := straight
	short.PunchHeight = 20
	if p := channel(t, 150, 60, 90).Sequence(short, 100); p.Found || len(p.Blocked) == 0 {
		t.Errorf("deep channel on a short punch: %+v, want no order", p)
	} else {
		for bi, r := range p.Blocked {
			if !r.Ram || r.Punch {
				t.Errorf("deep channel on a short punch, bend %d: %+v, want the ram and not the punch", bi, r)
			}
		}
	}
}

// TestOwnFlanges forms one bend, whose two flanges the punch must fit between: an 86° punch forms 90°, not 135°.
func TestOwnFlanges(t *testing.T) {
	for _, tt := range []struct {
		angle float64
		found bool
	}{{90, true}, {135, false}} {
		m, err := New(rect(100, 60), []Bend{{A: geometry.Point{X: 0, Y: 30}, B: geometry.Point{X: 100, Y: 30}, Angle: tt.angle, InsideRadius: 1.5, Up: true}}, 1.5, 0.44, 1)
		if err != nil {
			t.Fatal(err)
		}
		p := m.Sequence(straight, 100)
		if p.Found != tt.found {
			t.Errorf("%v° bend on an 86° punch: %+v, want found %v", tt.angle, p, tt.found)
		}
		if !tt.found && !p.Blocked[0].Punch {
			t.Errorf("%v° bend on an 86° punch: %+v, want the punch", tt.angle, p.Blocked[0])
		}
	}
	// A gooseneck's neck slopes back clear of the bend's own flanges at 90°.
	goose := Tooling{PunchTipAngle: 86, PunchWidth: 12, PunchHeight: 100, PunchNose: 12, PunchThroatDepth: 25, PunchThroatHeight: 60, DieOpening: 12, DieWidth: 15, DieHeight: 60}
	m, err := New(rect(100, 120), []Bend{along(60)}, 1.5, 0.44, 1)
	if err != nil {
		t.Fatal(err)
	}
	if p := m.Sequence(goose, 100); !p.Found {
		t.Errorf("90° bend on a gooseneck: %+v, want found", p)
	}
}

// BenchmarkSequence searches the bend order of a tray with four sides and a lip on two of them: six bends, about 5,000 samples.
func BenchmarkSequence(b *testing.B) {
	line := func(x0, y0, x1, y1 float64) (geometry.Point, geometry.Point) {
		return geometry.Point{X: x0, Y: y0}, geometry.Point{X: x1, Y: y1}
	}
	var bends []Bend
	for _, l := range [][4]float64{{0, 40, 200, 40}, {0, 160, 200, 160}, {40, 40, 40, 160}, {160, 40, 160, 160}, {0, 15, 200, 15}, {0, 185, 200, 185}} {
		a, c := line(l[0], l[1], l[2], l[3])
		bends = append(bends, Bend{A: a, B: c, Angle: 90, InsideRadius: 1.5, Up: true})
	}
	m, err := New(rect(200, 200), bends, 1.5, 0.44, 2.8)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		m.Sequence(straight, 1500)
	}
}

// TestPunchShape checks the punch cross-section: a straight punch is a tip and a body over the tip, a gooseneck carries its nose back at 45 degrees to a body set back behind it, away from the +y side, and mirrors to the other side.
func TestPunchShape(t *testing.T) {
	straight := Tooling{PunchTipAngle: 90, PunchWidth: 10, PunchHeight: 100}
	s := straight.punch(false)
	if len(s) != 2 || !s.near(0, 0, 0) || !s.near(4, 50, 0) || s.near(6, 50, 0) || s.near(0, -1, 0) {
		t.Errorf("straight punch: %d polygons, tip %v, body %v, beside %v, below %v", len(s), s.near(0, 0, 0), s.near(4, 50, 0), s.near(6, 50, 0), s.near(0, -1, 0))
	}
	goose := Tooling{PunchTipAngle: 90, PunchWidth: 10, PunchHeight: 100, PunchNose: 20, PunchThroatDepth: 30, PunchThroatHeight: 50}
	g := goose.punch(false)
	// The nose is under the tip; above it the throat clears the +y side; the neck slopes back from the nose's top (20) to the body (50), which stays set back by the throat's depth; behind the neck, below the body, is clear.
	checks := []struct {
		name string
		y, x float64
		in   bool
	}{{"nose", 4, 15, true}, {"+y above nose", 4, 40, false}, {"neck", -15, 35, true}, {"behind neck", -34, 40, false}, {"body", -34, 90, true}, {"+y shank", 4, 90, false}}
	for _, c := range checks {
		if g.near(c.y, c.x, 0) != c.in {
			t.Errorf("gooseneck %s (%v, %v): inside %v, want %v", c.name, c.y, c.x, !c.in, c.in)
		}
	}
	m := goose.punch(true)
	if !m.near(-4, 15, 0) || m.near(-4, 40, 0) || !m.near(15, 35, 0) || !m.near(34, 90, 0) {
		t.Errorf("mirrored gooseneck: nose %v, -y above nose %v, neck %v, body %v", m.near(-4, 15, 0), m.near(-4, 40, 0), m.near(15, 35, 0), m.near(34, 90, 0))
	}
	// The ram stands over the shank, above the punch.
	r := goose.ram(false)
	if !r.near(-34, 150, 0) || r.near(-34, 90, 0) || r.near(4, 150, 0) {
		t.Errorf("ram: over the shank %v, beside the body %v, over the throat %v", r.near(-34, 150, 0), r.near(-34, 90, 0), r.near(4, 150, 0))
	}
}
