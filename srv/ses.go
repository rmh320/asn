// Copyright 2014 Apptimist, Inc. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package srv

import (
	"crypto/rand"
	"github.com/apptimistco/asn"
	"os"
)

type Ses struct {
	ASN  *asn.ASN
	srv  *Server
	Keys struct {
		Server struct {
			Ephemeral asn.EncrPub
		}
		Client struct {
			Ephemeral, Login asn.EncrPub
		}
	}

	Lat, Lon, Range int32

	asnsrv bool // true if: asnsrv CONFIG ...
}

var SesPool chan *Ses

func init() { SesPool = make(chan *Ses, 16) }

func NewSes() (ses *Ses) {
	select {
	case ses = <-SesPool:
	default:
		ses = &Ses{}
	}
	ses.ASN = asn.NewASN()
	return
}

func SesPoolFlush() {
	for {
		select {
		case <-SesPool:
		default:
			return
		}
	}
}

func (ses *Ses) DN() string { return ses.srv.Config.Dir }

// Free the Ses by pooling or release it to GC if pool is full.
func (ses *Ses) Free() {
	if ses != nil {
		ses.ASN.Free()
		ses.ASN = nil
		ses.srv = nil
		select {
		case SesPool <- ses:
		default:
		}
	}
}

func (ses *Ses) IsAdmin(key *asn.EncrPub) bool {
	return *key == *ses.srv.Config.Keys.Admin.Pub.Encr
}

func (ses *Ses) IsService(key *asn.EncrPub) bool {
	return *key == *ses.srv.Config.Keys.Server.Pub.Encr
}

func (ses *Ses) Rekey(req asn.Requester) {
	var nonce asn.Nonce
	rand.Reader.Read(nonce[:])
	pub, sec, _ := asn.NewRandomEncrKeys()
	ses.Keys.Server.Ephemeral = *pub
	ses.ASN.Ack(req, pub[:], nonce[:])
	ses.ASN.SetStateEstablished()
	ses.ASN.SetBox(asn.NewBox(2, &nonce, &ses.Keys.Client.Ephemeral,
		pub, sec))
	ses.ASN.Println("rekeyed with", pub.String()[:8]+"...")
}

func (ses *Ses) RxBlob(pdu *asn.PDU) (err error) {
	blob, err := asn.NewBlobFrom(pdu)
	if err != nil {
		return
	}
	pdu.Rewind()
	sum := asn.NewSumOf(pdu)
	blobFN := asn.BlobFN(ses, sum)
	_, staterr := os.Stat(blobFN)
	if os.IsExist(staterr) {
		blob.Free()
		pdu.Free()
		return
	}
	if err = asn.Permission(ses, blob, &ses.Keys.Client.Login,
		&ses.Keys.Client.Ephemeral); err == nil {
		ses.ASN.Println("Blob", blob.Name)
		asn.MkReposPath(blobFN)
		pdu.SaveAs(blobFN)
		blob.Proc(ses, sum, blobFN, func(_ string, _ ...*asn.EncrPub) {
			// FIXME
		})
		blob = nil
		pdu = nil
	}
	return
}

func (ses *Ses) RxLogin(pdu *asn.PDU) (err error) {
	var req asn.Requester
	var sig asn.AuthSig
	req.ReadFrom(pdu)
	_, err = pdu.Read(ses.Keys.Client.Login[:])
	if err == nil {
		_, err = pdu.Read(sig[:])
	}
	if err != nil {
		return
	}
	err = asn.ErrFailure
	if ses.Keys.Client.Login.Equal(ses.srv.Config.Keys.Admin.Pub.Encr) {
		if sig.Verify(ses.srv.Config.Keys.Admin.Pub.Auth,
			ses.Keys.Client.Login[:]) {
			ses.ASN.Name = ses.srv.Config.Name + "[Admin]"
			err = nil
		}
	} else if ses.Keys.Client.Login.Equal(ses.srv.Config.Keys.Server.Pub.Encr) {
		if sig.Verify(ses.srv.Config.Keys.Server.Pub.Auth,
			ses.Keys.Client.Login[:]) {
			ses.ASN.Name = ses.srv.Config.Name + "[Server]"
			err = nil
		}
	} else {
		auth := asn.GetAsnAuth(ses, &ses.Keys.Client.Login)
		if sig.Verify(&auth, ses.Keys.Client.Login[:]) {
			ses.ASN.Name = ses.srv.Config.Name + "[" +
				ses.Keys.Client.Login.String()[:8] + "]"
			err = nil
		}
	}
	if err == nil {
		ses.ASN.Println("login")
		ses.Rekey(req)
		pdu.Free()
	} else {
		ses.ASN.Println("login", err)
	}
	return err
}

func (ses *Ses) RxPause(pdu *asn.PDU) error {
	var req asn.Requester
	req.ReadFrom(pdu)
	ses.ASN.Println("suspending")
	ses.ASN.Ack(req)
	ses.ASN.SetStateSuspended()
	pdu.Free()
	return nil
}

func (ses *Ses) RxQuit(pdu *asn.PDU) error {
	var req asn.Requester
	req.ReadFrom(pdu)
	ses.ASN.Println("quitting")
	ses.ASN.Ack(req)
	ses.ASN.SetStateQuitting()
	pdu.Free()
	return nil
}

func (ses *Ses) RxResume(pdu *asn.PDU) error {
	var req asn.Requester
	req.ReadFrom(pdu)
	ses.ASN.Println("resuming")
	ses.Rekey(req)
	pdu.Free()
	return nil
}

func (ses *Ses) Send(fn string, keys ...*asn.EncrPub) {
	// FIXME
}
