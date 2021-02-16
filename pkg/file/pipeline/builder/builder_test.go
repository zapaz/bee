// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package builder_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	mrand "math/rand"
	"strconv"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/file/joiner"
	"github.com/ethersphere/bee/pkg/file/pipeline/builder"
	test "github.com/ethersphere/bee/pkg/file/testing"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storage/mock"
	"github.com/ethersphere/bee/pkg/swarm"
	mockbytes "gitlab.com/nolash/go-mockbytes"
	"golang.org/x/crypto/sha3"
)

func TestPartialWrites(t *testing.T) {
	m := mock.NewStorer()
	p := builder.NewPipelineBuilder(context.Background(), m, storage.ModePutUpload, false)
	_, _ = p.Write([]byte("hello "))
	_, _ = p.Write([]byte("world"))

	sum, err := p.Sum()
	if err != nil {
		t.Fatal(err)
	}
	exp := swarm.MustParseHexAddress("92672a471f4419b255d7cb0cf313474a6f5856fb347c5ece85fb706d644b630f")
	if !bytes.Equal(exp.Bytes(), sum) {
		t.Fatalf("expected %s got %s", exp.String(), hex.EncodeToString(sum))
	}
}

func TestHelloWorld(t *testing.T) {
	m := mock.NewStorer()
	p := builder.NewPipelineBuilder(context.Background(), m, storage.ModePutUpload, false)

	data := []byte("hello world")
	_, err := p.Write(data)
	if err != nil {
		t.Fatal(err)
	}

	sum, err := p.Sum()
	if err != nil {
		t.Fatal(err)
	}
	exp := swarm.MustParseHexAddress("92672a471f4419b255d7cb0cf313474a6f5856fb347c5ece85fb706d644b630f")
	if !bytes.Equal(exp.Bytes(), sum) {
		t.Fatalf("expected %s got %s", exp.String(), hex.EncodeToString(sum))
	}
}

func TestAllVectors(t *testing.T) {
	for i := 1; i <= 20; i++ {
		data, expect := test.GetVector(t, i)
		t.Run(fmt.Sprintf("data length %d, vector %d", len(data), i), func(t *testing.T) {
			m := mock.NewStorer()
			p := builder.NewPipelineBuilder(context.Background(), m, storage.ModePutUpload, false)

			_, err := p.Write(data)
			if err != nil {
				t.Fatal(err)
			}
			sum, err := p.Sum()
			if err != nil {
				t.Fatal(err)
			}
			a := swarm.NewAddress(sum)
			if !a.Equal(expect) {
				t.Fatalf("failed run %d, expected address %s but got %s", i, expect.String(), a.String())
			}
		})
	}
}

func TestFindBug(t *testing.T) {
	m := mock.NewStorer()
	ctx := context.Background()
	for i := 128 * 128 * 4096; i <= 128*128*4096; i++ {
		//for i := 67100000; i <= 67100000; i++ {
		g := mockbytes.New(0, mockbytes.MockTypeStandard).WithModulus(255)
		data, err := g.SequentialBytes(i)
		if err != nil {
			t.Fatal(err)
		}
		t.Run(fmt.Sprintf("data length %d, vector %d", len(data), i), func(t *testing.T) {
			p := builder.NewPipelineBuilder(ctx, m, storage.ModePutUpload, false)

			_, err := p.Write(data)
			if err != nil {
				t.Fatal(err)
			}
			sum, err := p.Sum()
			if err != nil {
				t.Fatal(err)
			}
			a := swarm.NewAddress(sum)
			//fmt.Println("sum address", sum)
			j, l, err := joiner.New(ctx, m, a)
			if err != nil {
				t.Fatal(err)
			}

			if l != int64(i) {
				t.Fatalf("expected join data length %d, got %d", i, l)
			}
			joinData, err := ioutil.ReadAll(j)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(joinData, data) {
				t.Fatal("data mismatch")
				//t.Fatalf("retrieved data '%x' not like original data '%x'", joinData, data)
			}
		})
		if t.Failed() {
			return
		}
	}
}

func TestE2E(t *testing.T) {
	m := mock.NewStorer()
	ctx := context.Background()
	size := 100000000000                 // 100 gigs    // 128 * 128 * 128 * 4096
	buffer := make([]byte, 1024*1024*10) // ten megs buffer
	p := builder.NewPipelineBuilder(ctx, m, storage.ModePutUpload, false)
	r := mrand.New(mrand.NewSource(99))
	hasher := sha3.NewLegacyKeccak256()
	for written := 0; written < size; {
		ttt := time.Now()
		n, err := r.Read(buffer)
		if err != nil {
			t.Fatal(err)
		}
		fmt.Println("read took", time.Since(ttt))
		if n > size-written {
			n = size - written
		}
		_, err = p.Write(buffer[:n])
		if err != nil {
			t.Fatal(err)
		}
		_, err = hasher.Write(buffer[:n])
		if err != nil {
			t.Fatal(err)
		}
		written += n
	}
	sh3sum := hasher.Sum(nil) // sha3 sum
	fmt.Println(hex.EncodeToString(sh3sum))

	sum, err := p.Sum() // hashtrie sum
	if err != nil {
		t.Fatal(err)
	}
	a := swarm.NewAddress(sum)
	j, l, err := joiner.New(ctx, m, a)
	if err != nil {
		t.Fatal(err)
	}
	if l != int64(size) {
		t.Fatalf("expected join data length %d, got %d", size, l)
	}

	read := 0
	readHasher := sha3.NewLegacyKeccak256()

	for read < size {
		n, err := j.Read(buffer)
		if err != nil {
			t.Fatal(err)
		}
		_, err = readHasher.Write(buffer[:n])
		if err != nil {
			t.Fatal(err)
		}
		read += n
	}

	refSum := readHasher.Sum(nil)
	if !bytes.Equal(refSum, sh3sum) {
		t.Fatal("sums unequal!")
	}
}

/*
go test -v -bench=. -run Bench -benchmem
goos: linux
goarch: amd64
pkg: github.com/ethersphere/bee/pkg/file/pipeline/builder
BenchmarkPipeline
BenchmarkPipeline/1000-bytes
BenchmarkPipeline/1000-bytes-4         	   14475	     75170 ns/op	   63611 B/op	     333 allocs/op
BenchmarkPipeline/10000-bytes
BenchmarkPipeline/10000-bytes-4        	    2775	    459275 ns/op	  321825 B/op	    1826 allocs/op
BenchmarkPipeline/100000-bytes
BenchmarkPipeline/100000-bytes-4       	     334	   3523558 ns/op	 1891672 B/op	   11994 allocs/op
BenchmarkPipeline/1000000-bytes
BenchmarkPipeline/1000000-bytes-4      	      36	  33140883 ns/op	17745116 B/op	  114170 allocs/op
BenchmarkPipeline/10000000-bytes
BenchmarkPipeline/10000000-bytes-4     	       4	 304759595 ns/op	175378648 B/op	 1135082 allocs/op
BenchmarkPipeline/100000000-bytes
BenchmarkPipeline/100000000-bytes-4    	       1	3064439098 ns/op	1751509528 B/op	11342736 allocs/op
PASS
ok  	github.com/ethersphere/bee/pkg/file/pipeline/builder	17.599s

*/
func BenchmarkPipeline(b *testing.B) {
	for _, count := range []int{
		1000,      // 1k
		10000,     // 10 k
		100000,    // 100 k
		1000000,   // 1 mb
		10000000,  // 10 mb
		100000000, // 100 mb
	} {
		b.Run(strconv.Itoa(count)+"-bytes", func(b *testing.B) {
			for n := 0; n < b.N; n++ {
				benchmarkPipeline(b, count)
			}
		})
	}
}

func benchmarkPipeline(b *testing.B, count int) {
	b.StopTimer()

	m := mock.NewStorer()
	p := builder.NewPipelineBuilder(context.Background(), m, storage.ModePutUpload, false)
	data := make([]byte, count)
	_, err := rand.Read(data)
	if err != nil {
		b.Fatal(err)
	}

	b.StartTimer()

	_, err = p.Write(data)
	if err != nil {
		b.Fatal(err)
	}
	_, err = p.Sum()
	if err != nil {
		b.Fatal(err)
	}
}
