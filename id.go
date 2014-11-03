// Copyright 2014 Apptimist, Inc. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package asn

import (
	"io"
	"os"
)

type Id uint8

const (
	IdOff = VersionOff + int64(VersionSz)
	IdSz  = 1
)

const (
	RawId Id = iota

	AckReqId
	ExecReqId
	LoginReqId
	PauseReqId
	QuitReqId
	RedirectReqId
	ResumeReqId

	BlobId
	IndexId

	Nids

	IncompatibleId
	UnknownId

	MaxId = 16

	delFlag Id = 0x80
)

const (
	_ Id = iota

	AckReqV0
	ExecReqV0
	LoginReqV0
	PauseReqV0
	QuitReqV0
	RedirectReqV0
	ResumeReqV0

	BlobV0
	IndexV0
)

var (
	IdStrings = []string{
		RawId: "Raw",

		AckReqId:      "AckReq",
		ExecReqId:     "ExecReq",
		LoginReqId:    "LoginReq",
		PauseReqId:    "PauseReq",
		QuitReqId:     "QuitReq",
		RedirectReqId: "RedirectReq",
		ResumeReqId:   "ResumeReq",

		BlobId:  "Blob",
		IndexId: "Index",

		IncompatibleId: "Incompatible",
		UnknownId:      "Unknown",
	}

	VerId = [(Latest + 1) * MaxId]Id{
		((0 * MaxId) | AckReqV0):      AckReqId,
		((0 * MaxId) | ExecReqV0):     ExecReqId,
		((0 * MaxId) | LoginReqV0):    LoginReqId,
		((0 * MaxId) | PauseReqV0):    PauseReqId,
		((0 * MaxId) | QuitReqV0):     QuitReqId,
		((0 * MaxId) | RedirectReqV0): RedirectReqId,
		((0 * MaxId) | ResumeReqV0):   ResumeReqId,

		((0 * MaxId) | BlobV0):  BlobId,
		((0 * MaxId) | IndexV0): IndexId,
	}

	IdVer = [(Latest + 1) * MaxId]Id{
		((0 * MaxId) | AckReqId):      AckReqV0,
		((0 * MaxId) | ExecReqId):     ExecReqV0,
		((0 * MaxId) | LoginReqId):    LoginReqV0,
		((0 * MaxId) | PauseReqId):    PauseReqV0,
		((0 * MaxId) | QuitReqId):     QuitReqV0,
		((0 * MaxId) | RedirectReqId): RedirectReqV0,
		((0 * MaxId) | ResumeReqId):   ResumeReqV0,

		((0 * MaxId) | BlobId):  BlobV0,
		((0 * MaxId) | IndexId): IndexV0,
	}
)

// Internal Id from external Id of given version.
func (p *Id) Internal(v Version) {
	if v > Latest {
		*p = IncompatibleId
	} else if i := uint((uint(v) * MaxId) | uint(*p)); i > uint(Nids) {
		*p = UnknownId
	} else {
		*p = VerId[i]
		if *p == RawId {
			*p = UnknownId
		}
	}
}

// IsDeleted is true if Id is flaged for deletion by garbage collector
func (id Id) IsDeleted() bool {
	return (id & delFlag) != 0
}

// Flag for Deletion by garbage collector
func (p *Id) FlagDeletion() {
	*p = *p | delFlag
}

// UnFlag Deletion
func (p *Id) UnFlagDeletion() {
	*p = *p & ^delFlag
}

func (p *Id) ReadFrom(r io.Reader) (n int64, err error) {
	var b [1]byte
	ni, err := r.Read(b[:])
	if err == nil {
		n = int64(ni)
		*p = Id(b[0])
	}
	return
}

// String returns the name of internal Id.
func (id Id) String() string {
	i := int(id)
	if i >= len(IdStrings) {
		i = int(UnknownId)
	}
	return IdStrings[i]
}

// Version returns the given version of an Id.
func (id Id) Version(v Version) Id {
	if v > Latest {
		v = Latest
	}
	i := uint(v*MaxId) | uint(id)
	return IdVer[i]
}

func (id Id) WriteTo(w io.Writer) (n int64, err error) {
	b := []byte{byte(id)}
	ni, err := w.Write(b[:])
	if err == nil {
		n = int64(ni)
	}
	return
}

// Flag named file for Deletion by garbage collector
func FlagDeletion(name string) (err error) {
	return xFlagDeletion(name, func(id Id) Id {
		return id | delFlag
	})
}

// UnFlag named file for Deletion by garbage collector
func UnFlagDeletion(name string) (err error) {
	return xFlagDeletion(name, func(id Id) Id {
		return id & ^delFlag
	})
}

func xFlagDeletion(name string, x func(Id) Id) (err error) {
	var id Id
	f, err := os.OpenFile(name, os.O_RDWR, 0660)
	if err != nil {
		return
	}
	f.Seek(IdOff, os.SEEK_SET)
	id.ReadFrom(f)
	id = x(id)
	f.Seek(IdOff, os.SEEK_SET)
	id.WriteTo(f)
	f.Close()
	return
}

// IsDeleted is true if named object file is flaged for deletion
func IsDeleted(name string) bool {
	var id Id
	f, err := os.OpenFile(name, os.O_RDWR, 0660)
	if err != nil {
		return false
	}
	f.Seek(1, os.SEEK_SET)
	id.ReadFrom(f)
	f.Close()
	return id.IsDeleted()
}
