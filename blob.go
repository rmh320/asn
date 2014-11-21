// Copyright 2014 Apptimist, Inc. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package asn

import (
	"crypto/rand"
	"crypto/sha512"
	"errors"
	"fmt"
	"io"
	"os"
	"time"
)

const (
	BlobOff        = IdOff + int64(IdSz)
	BlobMagic      = "asnmagic"
	BlobMagicSz    = len(BlobMagic)
	BlobRandomSz   = 32
	BlobKeysSz     = 2 * EncrPubSz
	BlobTimeOff    = BlobOff + int64(BlobMagicSz+BlobRandomSz+BlobKeysSz)
	BlobTimeSz     = 8
	BlobNameLenOff = BlobTimeOff + int64(BlobTimeSz)
)

var (
	BlobPool    chan *Blob
	ErrNotMagic = errors.New("Not Magic")
)

func init() { BlobPool = make(chan *Blob, 16) }

// BlobTime seeks and reads time from named or opened file.
func BlobTime(v interface{}) (t time.Time) {
	var (
		f   *os.File
		fn  string
		err error
		ok  bool
	)
	if fn, ok = v.(string); ok {
		if f, err = os.Open(fn); err != nil {
			return
		}
		defer f.Close()
	} else if f, ok = v.(*os.File); !ok {
		return
	}
	f.Seek(BlobTimeOff, os.SEEK_SET)
	(NBOReader{f}).ReadNBO(&t)
	return
}

func ReadBlobContent(b []byte, fn string) (n int, err error) {
	f, err := os.Open(fn)
	if err != nil {
		return
	}
	defer f.Close()
	if _, err = SeekBlobContent(f); err != nil {
		return
	}
	n, err = f.Read(b)
	return
}

func ReadBlobKeyList(fn string) (keys []EncrPub, err error) {
	f, err := os.Open(fn)
	if err != nil {
		return
	}
	defer f.Close()
	pos, err := SeekBlobContent(f)
	if err != nil {
		return
	}
	fi, err := f.Stat()
	if err != nil {
		return
	}
	n := int(fi.Size()-pos) / EncrPubSz
	keys = make([]EncrPub, n)
	for i := 0; i < n; i++ {
		f.Read(keys[i][:])
	}
	return
}

// SeekBlobContent moves the ReadSeeker past the ASN blob headers
func SeekBlobContent(r io.ReadSeeker) (n int64, err error) {
	var b [1]byte
	n, err = r.Seek(BlobNameLenOff, os.SEEK_SET)
	if err != nil {
		return
	}
	_, err = r.Read(b[:])
	if err != nil {
		return
	}
	n, err = r.Seek(int64(b[0]), os.SEEK_CUR)
	return
}

type Blob struct {
	Owner  EncrPub
	Author EncrPub
	Time   time.Time
	Name   string
}

func BlobPoolFlush() {
	for {
		select {
		case <-BlobPool:
		default:
			return
		}
	}
}

func NewBlob(owner, author *EncrPub, name string) (blob *Blob) {
	select {
	case blob = <-BlobPool:
	default:
		blob = &Blob{}
	}
	blob.Owner = *owner
	blob.Author = *author
	blob.Name = name
	blob.Time = time.Now()
	return
}

func NewBlobFrom(r io.Reader) (blob *Blob, err error) {
	select {
	case blob = <-BlobPool:
	default:
		blob = &Blob{}
	}
	if _, err = blob.ReadFrom(r); err != nil {
		blob.Free()
		blob = nil
	}
	return
}

// FN returns a formatted file name of its time and abbreviated sum.
func (blob *Blob) FN(sum string) string {
	return fmt.Sprintf("%016x_%s", blob.Time.UnixNano(), sum[:16])
}

// Free the Blob by pooling or release it to GC if pool is full.
func (blob *Blob) Free() {
	if blob != nil {
		select {
		case BlobPool <- blob:
		default:
		}
	}
}

// Blob{}.ReadFrom *after* Id{}.ReadFrom(r)
func (blob *Blob) ReadFrom(r io.Reader) (n int64, err error) {
	var (
		b [256]byte
		x N
	)
	defer func() {
		n = int64(x)
	}()
	if err = x.Plus(r.Read(b[:BlobMagicSz])); err != nil {
		return
	}
	if string(b[:x]) != BlobMagic {
		err = ErrNotMagic
		return
	}
	if err = x.Plus(r.Read(b[:BlobRandomSz])); err != nil {
		return
	}
	if err = x.Plus(r.Read(blob.Owner[:])); err != nil {
		return
	}
	if err = x.Plus(r.Read(blob.Author[:])); err != nil {
		return
	}
	if err = x.Plus((NBOReader{r}).ReadNBO(&blob.Time)); err != nil {
		return
	}
	if err = x.Plus(r.Read(b[:1])); err != nil {
		return
	}
	if l := int(b[0]); l > 0 {
		if err = x.Plus(r.Read(b[:l])); err != nil {
			return
		}
		blob.Name = string(b[:l])
	} else {
		blob.Name = ""
	}
	return
}

// RFC822Z returns formatted time.
func (blob *Blob) RFC822Z() string { return blob.Time.Format(time.RFC822Z) }

// SummingWriteContentsTo writes a blob with v contents and returns it's sum
// along with bytes written and any error.
func (blob *Blob) SummingWriteContentsTo(w io.Writer, v interface{}) (sum *Sum,
	n int64, err error) {
	var (
		b [BlobRandomSz]byte
		x N
	)
	h := sha512.New()
	m := io.MultiWriter(w, h)
	defer func() {
		if err == nil {
			sum = new(Sum)
			copy(sum[:], h.Sum([]byte{}))
		}
		h.Reset()
		h = nil
		n = int64(x)
	}()
	if err = x.Plus(Latest.WriteTo(m)); err != nil {
		return
	}
	if err = x.Plus(BlobId.Version(Latest).WriteTo(m)); err != nil {
		return
	}
	if err = x.Plus(m.Write([]byte(BlobMagic))); err != nil {
		return
	}
	rand.Reader.Read(b[:BlobRandomSz])
	if err = x.Plus(m.Write(b[:BlobRandomSz])); err != nil {
		return
	}
	if err = x.Plus(m.Write(blob.Owner[:])); err != nil {
		return
	}
	if err = x.Plus(m.Write(blob.Author[:])); err != nil {
		return
	}
	if err = x.Plus((NBOWriter{m}).WriteNBO(blob.Time)); err != nil {
		return
	}
	b[0] = byte(len(blob.Name))
	if err = x.Plus(m.Write(b[:1])); err != nil {
		return
	}
	if b[0] > 0 {
		if err = x.Plus(m.Write([]byte(blob.Name[:]))); err != nil {
			return
		}
	}
	switch t := v.(type) {
	case Mark:
		err = x.Plus(t.WriteTo(m))
	case Sums:
		err = x.Plus(t.WriteTo(m))
	case []byte:
		err = x.Plus(m.Write(t))
	case string:
		err = x.Plus(m.Write([]byte(t)))
	case io.Reader:
		err = x.Plus(io.Copy(m, t))
	}
	return
}