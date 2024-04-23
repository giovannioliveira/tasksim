// Copyright 2022 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build ignore

package main

// Running this generator requires addchain v0.4.0, which can be installed with
//
//   go install github.com/mmcloughlin/addchain/cmd/addchain@v0.4.0
//

import (
	"bytes"
	"crypto/elliptic"
	"fmt"
	"go/format"
	"io"
	"log"
	"math/big"
	"os"
	"os/exec"
	"strings"
	"text/template"
)

var curves = []struct {
	P         string
	Element   string
	Params    *elliptic.CurveParams
	BuildTags string
}{
	{
		P:       "P224",
		Element: "fiat.P224Element",
		Params:  elliptic.P224().Params(),
	},
	{
		P:         "P256",
		Element:   "fiat.P256Element",
		Params:    elliptic.P256().Params(),
		BuildTags: "!amd64 && !arm64 && !ppc64le && !s390x",
	},
	{
		P:       "P384",
		Element: "fiat.P384Element",
		Params:  elliptic.P384().Params(),
	},
	{
		P:       "P521",
		Element: "fiat.P521Element",
		Params:  elliptic.P521().Params(),
	},
}

func main() {
	t := template.Must(template.New("tmplNISTEC").Parse(tmplNISTEC))

	tmplAddchainFile, err := os.CreateTemp("", "addchain-template")
	if err != nil {
		log.Fatal(err)
	}
	defer os.Remove(tmplAddchainFile.Name())
	if _, err := io.WriteString(tmplAddchainFile, tmplAddchain); err != nil {
		log.Fatal(err)
	}
	if err := tmplAddchainFile.Close(); err != nil {
		log.Fatal(err)
	}

	for _, c := range curves {
		p := strings.ToLower(c.P)
		elementLen := (c.Params.BitSize + 7) / 8
		B := fmt.Sprintf("%#v", c.Params.B.FillBytes(make([]byte, elementLen)))
		Gx := fmt.Sprintf("%#v", c.Params.Gx.FillBytes(make([]byte, elementLen)))
		Gy := fmt.Sprintf("%#v", c.Params.Gy.FillBytes(make([]byte, elementLen)))

		log.Printf("Generating %s.go...", p)
		f, err := os.Create(p + ".go")
		if err != nil {
			log.Fatal(err)
		}
		defer f.Close()
		buf := &bytes.Buffer{}
		if err := t.Execute(buf, map[string]interface{}{
			"P": c.P, "p": p, "B": B, "Gx": Gx, "Gy": Gy,
			"Element": c.Element, "ElementLen": elementLen,
			"BuildTags": c.BuildTags,
		}); err != nil {
			log.Fatal(err)
		}
		out, err := format.Source(buf.Bytes())
		if err != nil {
			log.Fatal(err)
		}
		if _, err := f.Write(out); err != nil {
			log.Fatal(err)
		}

		// If p = 3 mod 4, implement modular square root by exponentiation.
		mod4 := new(big.Int).Mod(c.Params.P, big.NewInt(4))
		if mod4.Cmp(big.NewInt(3)) != 0 {
			continue
		}

		exp := new(big.Int).Add(c.Params.P, big.NewInt(1))
		exp.Div(exp, big.NewInt(4))

		tmp, err := os.CreateTemp("", "addchain-"+p)
		if err != nil {
			log.Fatal(err)
		}
		defer os.Remove(tmp.Name())
		cmd := exec.Command("addchain", "search", fmt.Sprintf("%d", exp))
		cmd.Stderr = os.Stderr
		cmd.Stdout = tmp
		if err := cmd.Run(); err != nil {
			log.Fatal(err)
		}
		if err := tmp.Close(); err != nil {
			log.Fatal(err)
		}
		cmd = exec.Command("addchain", "gen", "-tmpl", tmplAddchainFile.Name(), tmp.Name())
		cmd.Stderr = os.Stderr
		out, err = cmd.Output()
		if err != nil {
			log.Fatal(err)
		}
		out = bytes.Replace(out, []byte("Element"), []byte(c.Element), -1)
		out = bytes.Replace(out, []byte("sqrtCandidate"), []byte(p+"SqrtCandidate"), -1)
		out, err = format.Source(out)
		if err != nil {
			log.Fatal(err)
		}
		if _, err := f.Write(out); err != nil {
			log.Fatal(err)
		}
	}
}

const tmplNISTEC = `// Copyright 2022 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Code generated by generate.go. DO NOT EDIT.

{{ if .BuildTags }}
//go:build {{ .BuildTags }}
{{ end }}

package nistec

import (
	"crypto/internal/nistec/fiat"
	"crypto/subtle"
	"errors"
	"sync"
)

// {{.p}}ElementLength is the length of an element of the base or scalar field,
// which have the same bytes length for all NIST P curves.
const {{.p}}ElementLength = {{ .ElementLen }}

// {{.P}}Point is a {{.P}} point. The zero value is NOT valid.
type {{.P}}Point struct {
	// The point is represented in projective coordinates (X:Y:Z),
	// where x = X/Z and y = Y/Z.
	x, y, z *{{.Element}}
}

// New{{.P}}Point returns a new {{.P}}Point representing the point at infinity point.
func New{{.P}}Point() *{{.P}}Point {
	return &{{.P}}Point{
		x: new({{.Element}}),
		y: new({{.Element}}).One(),
		z: new({{.Element}}),
	}
}

// SetGenerator sets p to the canonical generator and returns p.
func (p *{{.P}}Point) SetGenerator() *{{.P}}Point {
	p.x.SetBytes({{.Gx}})
	p.y.SetBytes({{.Gy}})
	p.z.One()
	return p
}

// Set sets p = q and returns p.
func (p *{{.P}}Point) Set(q *{{.P}}Point) *{{.P}}Point {
	p.x.Set(q.x)
	p.y.Set(q.y)
	p.z.Set(q.z)
	return p
}

// SetBytes sets p to the compressed, uncompressed, or infinity value encoded in
// b, as specified in SEC 1, Version 2.0, Section 2.3.4. If the point is not on
// the curve, it returns nil and an error, and the receiver is unchanged.
// Otherwise, it returns p.
func (p *{{.P}}Point) SetBytes(b []byte) (*{{.P}}Point, error) {
	switch {
	// Point at infinity.
	case len(b) == 1 && b[0] == 0:
		return p.Set(New{{.P}}Point()), nil

	// Uncompressed form.
	case len(b) == 1+2*{{.p}}ElementLength && b[0] == 4:
		x, err := new({{.Element}}).SetBytes(b[1 : 1+{{.p}}ElementLength])
		if err != nil {
			return nil, err
		}
		y, err := new({{.Element}}).SetBytes(b[1+{{.p}}ElementLength:])
		if err != nil {
			return nil, err
		}
		if err := {{.p}}CheckOnCurve(x, y); err != nil {
			return nil, err
		}
		p.x.Set(x)
		p.y.Set(y)
		p.z.One()
		return p, nil

	// Compressed form.
	case len(b) == 1+{{.p}}ElementLength && (b[0] == 2 || b[0] == 3):
		x, err := new({{.Element}}).SetBytes(b[1:])
		if err != nil {
			return nil, err
		}

		// y² = x³ - 3x + b
		y := {{.p}}Polynomial(new({{.Element}}), x)
		if !{{.p}}Sqrt(y, y) {
			return nil, errors.New("invalid {{.P}} compressed point encoding")
		}

		// Select the positive or negative root, as indicated by the least
		// significant bit, based on the encoding type byte.
		otherRoot := new({{.Element}})
		otherRoot.Sub(otherRoot, y)
		cond := y.Bytes()[{{.p}}ElementLength-1]&1 ^ b[0]&1
		y.Select(otherRoot, y, int(cond))

		p.x.Set(x)
		p.y.Set(y)
		p.z.One()
		return p, nil

	default:
		return nil, errors.New("invalid {{.P}} point encoding")
	}
}


var _{{.p}}B *{{.Element}}
var _{{.p}}BOnce sync.Once

func {{.p}}B() *{{.Element}} {
	_{{.p}}BOnce.Do(func() {
		_{{.p}}B, _ = new({{.Element}}).SetBytes({{.B}})
	})
	return _{{.p}}B
}

// {{.p}}Polynomial sets y2 to x³ - 3x + b, and returns y2.
func {{.p}}Polynomial(y2, x *{{.Element}}) *{{.Element}} {
	y2.Square(x)
	y2.Mul(y2, x)

	threeX := new({{.Element}}).Add(x, x)
	threeX.Add(threeX, x)
	y2.Sub(y2, threeX)

	return y2.Add(y2, {{.p}}B())
}

func {{.p}}CheckOnCurve(x, y *{{.Element}}) error {
	// y² = x³ - 3x + b
	rhs := {{.p}}Polynomial(new({{.Element}}), x)
	lhs := new({{.Element}}).Square(y)
	if rhs.Equal(lhs) != 1 {
		return errors.New("{{.P}} point not on curve")
	}
	return nil
}

// Bytes returns the uncompressed or infinity encoding of p, as specified in
// SEC 1, Version 2.0, Section 2.3.3. Note that the encoding of the point at
// infinity is shorter than all other encodings.
func (p *{{.P}}Point) Bytes() []byte {
	// This function is outlined to make the allocations inline in the caller
	// rather than happen on the heap.
	var out [1+2*{{.p}}ElementLength]byte
	return p.bytes(&out)
}

func (p *{{.P}}Point) bytes(out *[1+2*{{.p}}ElementLength]byte) []byte {
	if p.z.IsZero() == 1 {
		return append(out[:0], 0)
	}

	zinv := new({{.Element}}).Invert(p.z)
	x := new({{.Element}}).Mul(p.x, zinv)
	y := new({{.Element}}).Mul(p.y, zinv)

	buf := append(out[:0], 4)
	buf = append(buf, x.Bytes()...)
	buf = append(buf, y.Bytes()...)
	return buf
}

// BytesX returns the encoding of the x-coordinate of p, as specified in SEC 1,
// Version 2.0, Section 2.3.5, or an error if p is the point at infinity.
func (p *{{.P}}Point) BytesX() ([]byte, error) {
	// This function is outlined to make the allocations inline in the caller
	// rather than happen on the heap.
	var out [{{.p}}ElementLength]byte
	return p.bytesX(&out)
}

func (p *{{.P}}Point) bytesX(out *[{{.p}}ElementLength]byte) ([]byte, error) {
	if p.z.IsZero() == 1 {
		return nil, errors.New("{{.P}} point is the point at infinity")
	}

	zinv := new({{.Element}}).Invert(p.z)
	x := new({{.Element}}).Mul(p.x, zinv)

	return append(out[:0], x.Bytes()...), nil
}

// BytesCompressed returns the compressed or infinity encoding of p, as
// specified in SEC 1, Version 2.0, Section 2.3.3. Note that the encoding of the
// point at infinity is shorter than all other encodings.
func (p *{{.P}}Point) BytesCompressed() []byte {
	// This function is outlined to make the allocations inline in the caller
	// rather than happen on the heap.
	var out [1 + {{.p}}ElementLength]byte
	return p.bytesCompressed(&out)
}

func (p *{{.P}}Point) bytesCompressed(out *[1 + {{.p}}ElementLength]byte) []byte {
	if p.z.IsZero() == 1 {
		return append(out[:0], 0)
	}

	zinv := new({{.Element}}).Invert(p.z)
	x := new({{.Element}}).Mul(p.x, zinv)
	y := new({{.Element}}).Mul(p.y, zinv)

	// Encode the sign of the y coordinate (indicated by the least significant
	// bit) as the encoding type (2 or 3).
	buf := append(out[:0], 2)
	buf[0] |= y.Bytes()[{{.p}}ElementLength-1] & 1
	buf = append(buf, x.Bytes()...)
	return buf
}

// Add sets q = p1 + p2, and returns q. The points may overlap.
func (q *{{.P}}Point) Add(p1, p2 *{{.P}}Point) *{{.P}}Point {
	// Complete addition formula for a = -3 from "Complete addition formulas for
	// prime order elliptic curves" (https://eprint.iacr.org/2015/1060), §A.2.

	t0 := new({{.Element}}).Mul(p1.x, p2.x)   // t0 := X1 * X2
	t1 := new({{.Element}}).Mul(p1.y, p2.y)   // t1 := Y1 * Y2
	t2 := new({{.Element}}).Mul(p1.z, p2.z)   // t2 := Z1 * Z2
	t3 := new({{.Element}}).Add(p1.x, p1.y)   // t3 := X1 + Y1
	t4 := new({{.Element}}).Add(p2.x, p2.y)   // t4 := X2 + Y2
	t3.Mul(t3, t4)                            // t3 := t3 * t4
	t4.Add(t0, t1)                            // t4 := t0 + t1
	t3.Sub(t3, t4)                            // t3 := t3 - t4
	t4.Add(p1.y, p1.z)                        // t4 := Y1 + Z1
	x3 := new({{.Element}}).Add(p2.y, p2.z)   // X3 := Y2 + Z2
	t4.Mul(t4, x3)                            // t4 := t4 * X3
	x3.Add(t1, t2)                            // X3 := t1 + t2
	t4.Sub(t4, x3)                            // t4 := t4 - X3
	x3.Add(p1.x, p1.z)                        // X3 := X1 + Z1
	y3 := new({{.Element}}).Add(p2.x, p2.z)   // Y3 := X2 + Z2
	x3.Mul(x3, y3)                            // X3 := X3 * Y3
	y3.Add(t0, t2)                            // Y3 := t0 + t2
	y3.Sub(x3, y3)                            // Y3 := X3 - Y3
	z3 := new({{.Element}}).Mul({{.p}}B(), t2)  // Z3 := b * t2
	x3.Sub(y3, z3)                            // X3 := Y3 - Z3
	z3.Add(x3, x3)                            // Z3 := X3 + X3
	x3.Add(x3, z3)                            // X3 := X3 + Z3
	z3.Sub(t1, x3)                            // Z3 := t1 - X3
	x3.Add(t1, x3)                            // X3 := t1 + X3
	y3.Mul({{.p}}B(), y3)                     // Y3 := b * Y3
	t1.Add(t2, t2)                            // t1 := t2 + t2
	t2.Add(t1, t2)                            // t2 := t1 + t2
	y3.Sub(y3, t2)                            // Y3 := Y3 - t2
	y3.Sub(y3, t0)                            // Y3 := Y3 - t0
	t1.Add(y3, y3)                            // t1 := Y3 + Y3
	y3.Add(t1, y3)                            // Y3 := t1 + Y3
	t1.Add(t0, t0)                            // t1 := t0 + t0
	t0.Add(t1, t0)                            // t0 := t1 + t0
	t0.Sub(t0, t2)                            // t0 := t0 - t2
	t1.Mul(t4, y3)                            // t1 := t4 * Y3
	t2.Mul(t0, y3)                            // t2 := t0 * Y3
	y3.Mul(x3, z3)                            // Y3 := X3 * Z3
	y3.Add(y3, t2)                            // Y3 := Y3 + t2
	x3.Mul(t3, x3)                            // X3 := t3 * X3
	x3.Sub(x3, t1)                            // X3 := X3 - t1
	z3.Mul(t4, z3)                            // Z3 := t4 * Z3
	t1.Mul(t3, t0)                            // t1 := t3 * t0
	z3.Add(z3, t1)                            // Z3 := Z3 + t1

	q.x.Set(x3)
	q.y.Set(y3)
	q.z.Set(z3)
	return q
}

// Double sets q = p + p, and returns q. The points may overlap.
func (q *{{.P}}Point) Double(p *{{.P}}Point) *{{.P}}Point {
	// Complete addition formula for a = -3 from "Complete addition formulas for
	// prime order elliptic curves" (https://eprint.iacr.org/2015/1060), §A.2.

	t0 := new({{.Element}}).Square(p.x)      // t0 := X ^ 2
	t1 := new({{.Element}}).Square(p.y)      // t1 := Y ^ 2
	t2 := new({{.Element}}).Square(p.z)      // t2 := Z ^ 2
	t3 := new({{.Element}}).Mul(p.x, p.y)    // t3 := X * Y
	t3.Add(t3, t3)                           // t3 := t3 + t3
	z3 := new({{.Element}}).Mul(p.x, p.z)    // Z3 := X * Z
	z3.Add(z3, z3)                           // Z3 := Z3 + Z3
	y3 := new({{.Element}}).Mul({{.p}}B(), t2) // Y3 := b * t2
	y3.Sub(y3, z3)                           // Y3 := Y3 - Z3
	x3 := new({{.Element}}).Add(y3, y3)      // X3 := Y3 + Y3
	y3.Add(x3, y3)                           // Y3 := X3 + Y3
	x3.Sub(t1, y3)                           // X3 := t1 - Y3
	y3.Add(t1, y3)                           // Y3 := t1 + Y3
	y3.Mul(x3, y3)                           // Y3 := X3 * Y3
	x3.Mul(x3, t3)                           // X3 := X3 * t3
	t3.Add(t2, t2)                           // t3 := t2 + t2
	t2.Add(t2, t3)                           // t2 := t2 + t3
	z3.Mul({{.p}}B(), z3)                    // Z3 := b * Z3
	z3.Sub(z3, t2)                           // Z3 := Z3 - t2
	z3.Sub(z3, t0)                           // Z3 := Z3 - t0
	t3.Add(z3, z3)                           // t3 := Z3 + Z3
	z3.Add(z3, t3)                           // Z3 := Z3 + t3
	t3.Add(t0, t0)                           // t3 := t0 + t0
	t0.Add(t3, t0)                           // t0 := t3 + t0
	t0.Sub(t0, t2)                           // t0 := t0 - t2
	t0.Mul(t0, z3)                           // t0 := t0 * Z3
	y3.Add(y3, t0)                           // Y3 := Y3 + t0
	t0.Mul(p.y, p.z)                         // t0 := Y * Z
	t0.Add(t0, t0)                           // t0 := t0 + t0
	z3.Mul(t0, z3)                           // Z3 := t0 * Z3
	x3.Sub(x3, z3)                           // X3 := X3 - Z3
	z3.Mul(t0, t1)                           // Z3 := t0 * t1
	z3.Add(z3, z3)                           // Z3 := Z3 + Z3
	z3.Add(z3, z3)                           // Z3 := Z3 + Z3

	q.x.Set(x3)
	q.y.Set(y3)
	q.z.Set(z3)
	return q
}

// Select sets q to p1 if cond == 1, and to p2 if cond == 0.
func (q *{{.P}}Point) Select(p1, p2 *{{.P}}Point, cond int) *{{.P}}Point {
	q.x.Select(p1.x, p2.x, cond)
	q.y.Select(p1.y, p2.y, cond)
	q.z.Select(p1.z, p2.z, cond)
	return q
}

// A {{.p}}Table holds the first 15 multiples of a point at offset -1, so [1]P
// is at table[0], [15]P is at table[14], and [0]P is implicitly the identity
// point.
type {{.p}}Table [15]*{{.P}}Point

// Select selects the n-th multiple of the table base point into p. It works in
// constant time by iterating over every entry of the table. n must be in [0, 15].
func (table *{{.p}}Table) Select(p *{{.P}}Point, n uint8) {
	if n >= 16 {
		panic("nistec: internal error: {{.p}}Table called with out-of-bounds value")
	}
	p.Set(New{{.P}}Point())
	for i := uint8(1); i < 16; i++ {
		cond := subtle.ConstantTimeByteEq(i, n)
		p.Select(table[i-1], p, cond)
	}
}

// ScalarMult sets p = scalar * q, and returns p.
func (p *{{.P}}Point) ScalarMult(q *{{.P}}Point, scalar []byte) (*{{.P}}Point, error) {
	// Compute a {{.p}}Table for the base point q. The explicit New{{.P}}Point
	// calls get inlined, letting the allocations live on the stack.
	var table = {{.p}}Table{New{{.P}}Point(), New{{.P}}Point(), New{{.P}}Point(),
		New{{.P}}Point(), New{{.P}}Point(), New{{.P}}Point(), New{{.P}}Point(),
		New{{.P}}Point(), New{{.P}}Point(), New{{.P}}Point(), New{{.P}}Point(),
		New{{.P}}Point(), New{{.P}}Point(), New{{.P}}Point(), New{{.P}}Point()}
	table[0].Set(q)
	for i := 1; i < 15; i += 2 {
		table[i].Double(table[i/2])
		table[i+1].Add(table[i], q)
	}

	// Instead of doing the classic double-and-add chain, we do it with a
	// four-bit window: we double four times, and then add [0-15]P.
	t := New{{.P}}Point()
	p.Set(New{{.P}}Point())
	for i, byte := range scalar {
		// No need to double on the first iteration, as p is the identity at
		// this point, and [N]∞ = ∞.
		if i != 0 {
			p.Double(p)
			p.Double(p)
			p.Double(p)
			p.Double(p)
		}

		windowValue := byte >> 4
		table.Select(t, windowValue)
		p.Add(p, t)

		p.Double(p)
		p.Double(p)
		p.Double(p)
		p.Double(p)

		windowValue = byte & 0b1111
		table.Select(t, windowValue)
		p.Add(p, t)
	}

	return p, nil
}

var {{.p}}GeneratorTable *[{{.p}}ElementLength * 2]{{.p}}Table
var {{.p}}GeneratorTableOnce sync.Once

// generatorTable returns a sequence of {{.p}}Tables. The first table contains
// multiples of G. Each successive table is the previous table doubled four
// times.
func (p *{{.P}}Point) generatorTable() *[{{.p}}ElementLength * 2]{{.p}}Table {
	{{.p}}GeneratorTableOnce.Do(func() {
		{{.p}}GeneratorTable = new([{{.p}}ElementLength * 2]{{.p}}Table)
		base := New{{.P}}Point().SetGenerator()
		for i := 0; i < {{.p}}ElementLength*2; i++ {
			{{.p}}GeneratorTable[i][0] = New{{.P}}Point().Set(base)
			for j := 1; j < 15; j++ {
				{{.p}}GeneratorTable[i][j] = New{{.P}}Point().Add({{.p}}GeneratorTable[i][j-1], base)
			}
			base.Double(base)
			base.Double(base)
			base.Double(base)
			base.Double(base)
		}
	})
	return {{.p}}GeneratorTable
}

// ScalarBaseMult sets p = scalar * B, where B is the canonical generator, and
// returns p.
func (p *{{.P}}Point) ScalarBaseMult(scalar []byte) (*{{.P}}Point, error) {
	if len(scalar) != {{.p}}ElementLength {
		return nil, errors.New("invalid scalar length")
	}
	tables := p.generatorTable()

	// This is also a scalar multiplication with a four-bit window like in
	// ScalarMult, but in this case the doublings are precomputed. The value
	// [windowValue]G added at iteration k would normally get doubled
	// (totIterations-k)×4 times, but with a larger precomputation we can
	// instead add [2^((totIterations-k)×4)][windowValue]G and avoid the
	// doublings between iterations.
	t := New{{.P}}Point()
	p.Set(New{{.P}}Point())
	tableIndex := len(tables) - 1
	for _, byte := range scalar {
		windowValue := byte >> 4
		tables[tableIndex].Select(t, windowValue)
		p.Add(p, t)
		tableIndex--

		windowValue = byte & 0b1111
		tables[tableIndex].Select(t, windowValue)
		p.Add(p, t)
		tableIndex--
	}

	return p, nil
}

// {{.p}}Sqrt sets e to a square root of x. If x is not a square, {{.p}}Sqrt returns
// false and e is unchanged. e and x can overlap.
func {{.p}}Sqrt(e, x *{{ .Element }}) (isSquare bool) {
	candidate := new({{ .Element }})
	{{.p}}SqrtCandidate(candidate, x)
	square := new({{ .Element }}).Square(candidate)
	if square.Equal(x) != 1 {
		return false
	}
	e.Set(candidate)
	return true
}
`

const tmplAddchain = `
// sqrtCandidate sets z to a square root candidate for x. z and x must not overlap.
func sqrtCandidate(z, x *Element) {
	// Since p = 3 mod 4, exponentiation by (p + 1) / 4 yields a square root candidate.
	//
	// The sequence of {{ .Ops.Adds }} multiplications and {{ .Ops.Doubles }} squarings is derived from the
	// following addition chain generated with {{ .Meta.Module }} {{ .Meta.ReleaseTag }}.
	//
	{{- range lines (format .Script) }}
	//	{{ . }}
	{{- end }}
	//

	{{- range .Program.Temporaries }}
	var {{ . }} = new(Element)
	{{- end }}
	{{ range $i := .Program.Instructions -}}
	{{- with add $i.Op }}
	{{ $i.Output }}.Mul({{ .X }}, {{ .Y }})
	{{- end -}}

	{{- with double $i.Op }}
	{{ $i.Output }}.Square({{ .X }})
	{{- end -}}

	{{- with shift $i.Op -}}
	{{- $first := 0 -}}
	{{- if ne $i.Output.Identifier .X.Identifier }}
	{{ $i.Output }}.Square({{ .X }})
	{{- $first = 1 -}}
	{{- end }}
	for s := {{ $first }}; s < {{ .S }}; s++ {
		{{ $i.Output }}.Square({{ $i.Output }})
	}
	{{- end -}}
	{{- end }}
}
`
