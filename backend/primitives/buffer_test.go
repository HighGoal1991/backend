package primitives

import (
	"encoding/json"
	"io/ioutil"
	"math/rand"
	"testing"
)

func TestRowColLineWord(t *testing.T) {
	var b Buffer
	if d, err := ioutil.ReadFile("./testdata/unittest.json"); err != nil {
		t.Fatal(err)
	} else {
		type Test struct {
			Offset           int
			Line             int
			Column           int
			LineAtOffset     Region
			WordAtOffset     Region
			FullLineAtOffset Region
		}
		var tests []Test
		if err := json.Unmarshal(d, &tests); err != nil {
			t.Fatal(err)
		} else {
			if d, err := ioutil.ReadFile("./testdata/unittest.cpp"); err != nil {
				t.Fatal(err)
			} else {
				b.Insert(0, string(d))
			}

			failed := 0
			// TODO. Currently all but this count of the test matrix is equal to ST3, which is better than nothing
			const expected = 97
			for i, test := range tests {
				var a Test
				a.Line, a.Column = b.RowCol(test.Offset)
				a.LineAtOffset = b.Line(test.Offset)
				a.WordAtOffset = b.Word(test.Offset)
				a.FullLineAtOffset = b.FullLine(test.Offset)
				a.Offset = b.TextPoint(test.Line, test.Column)
				//				t.Log(a)
				if a.Line != test.Line {
					failed++
					t.Logf("%d Line mismatch: %d != %d", i, a.Line, test.Line)
				}
				if a.Column != test.Column {
					failed++
					t.Logf("%d Column mismatch: %d != %d", i, a.Column, test.Column)
				}
				if a.Offset != test.Offset {
					failed++
					t.Logf("%d Offset mismatch: %d != %d", i, a.Offset, test.Offset)
				}
				if a.LineAtOffset != test.LineAtOffset {
					failed++
					t.Logf("%d LineAtOffset mismatch: '%s' != '%s', '%s' != '%s'", i, a.LineAtOffset, test.LineAtOffset, b.Substr(a.LineAtOffset), b.Substr(test.LineAtOffset))
				}
				if a.FullLineAtOffset != test.FullLineAtOffset {
					failed++
					t.Logf("%d FullLineAtOffset mismatch: '%s' != '%s', '%s' != '%s'", i, a.FullLineAtOffset, test.FullLineAtOffset, b.Substr(a.FullLineAtOffset), b.Substr(test.FullLineAtOffset))
				}
				if a.WordAtOffset != test.WordAtOffset {
					failed++
					t.Logf("%d WordAtOffset mismatch: '%s' != '%s', '%s' != '%s'", i, a.WordAtOffset, test.WordAtOffset, b.Substr(a.WordAtOffset), b.Substr(test.WordAtOffset))
				}
			}
			t.Logf("%d/%d= %f%% passing", failed, len(tests), 100*(float64(len(tests))-float64(failed))/(float64(len(tests))))
			if failed != expected {
				t.Errorf("Expected %d tests to fail, not %d", expected, failed)
			}
		}
	}
	if r, c := b.RowCol(-1); r != 0 || c != 0 {
		t.Errorf("These should be 0 %d, %d", r, c)
	}
	if r, c := b.RowCol(b.Size() + 10); c != 0 {
		t.Errorf("Column should be 0 %d, %d", r, c)
	}
}

func TestSomething(t *testing.T) {
	var b Buffer
	b.Insert(0, "testaråäöochliteannat€þıœəßðĸʒ×ŋµåäö𝄞")
	t.Log(b.Line(0))
	t.Log(b.Word(3))
	t.Log(b.Word(7))
	t.Log(b.Word(11))
	t.Log(b.Word(12))
}

func fill(data []rune) {
	s := int('a')
	e := int('z')
	l := int(e - s)
	for i := range data {
		data[i] = rune(s + (rand.Int() % l))
	}
}

const (
	testbuffer_size = 1024 * 1024
	testinsert_size = 1
)

func testinsert() string {
	data := make([]rune, 1)
	fill(data)
	return string(data)
}

func testbuffer() *Buffer {
	var buf Buffer
	buf.data = make([]rune, testbuffer_size)
	fill(buf.data)
	return &buf
}

func BenchmarkSubstr(b *testing.B) {
	b.StopTimer()
	r := rand.Perm(b.N)
	buf := testbuffer()
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		buf.Substr(buf.Word(r[i]))
	}
}

func BenchmarkSubstrRunes(b *testing.B) {
	b.StopTimer()
	r := rand.Perm(b.N)
	buf := testbuffer()
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		rr := buf.Word(r[i])
		_ = buf.data[rr.A:rr.B]
	}
}

func BenchmarkWord(b *testing.B) {
	b.StopTimer()
	r := rand.Perm(b.N)
	buf := testbuffer()
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		buf.Word(r[i])
	}
}

func BenchmarkLine(b *testing.B) {
	b.StopTimer()
	r := rand.Perm(b.N)
	buf := testbuffer()
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		buf.Line(r[i])
	}
}

func BenchmarkRowCol(b *testing.B) {
	b.StopTimer()
	r := rand.Perm(b.N)
	buf := testbuffer()
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		buf.RowCol(r[i])
	}
}

func BenchmarkBufferInsertRand(b *testing.B) {
	b.StopTimer()
	sdata := testinsert()
	r := rand.Perm(b.N)
	buf := testbuffer()
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		buf.Insert(r[i], sdata)
	}
}

func BenchmarkBufferInsertBegin(b *testing.B) {
	b.StopTimer()
	sdata := testinsert()
	buf := testbuffer()
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		buf.Insert(0, sdata)
	}
}

func BenchmarkBufferInsertMid(b *testing.B) {
	b.StopTimer()
	sdata := testinsert()
	buf := testbuffer()
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		buf.Insert(buf.Size()/2, sdata)
	}
}

func BenchmarkBufferInsertEnd(b *testing.B) {
	b.StopTimer()
	sdata := testinsert()
	buf := testbuffer()
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		buf.Insert(buf.Size(), sdata)
	}
}
