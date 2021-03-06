// This file is part of the GOfax.IP project - https://github.com/gonicus/gofaxip
// Copyright (C) 2014 GONICUS GmbH, Germany - http://www.gonicus.de
//
// This program is free software; you can redistribute it and/or
// modify it under the terms of the GNU General Public License
// as published by the Free Software Foundation; version 2
// of the License.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program; if not, write to the Free Software
// Foundation, Inc., 51 Franklin Street, Fifth Floor, Boston, MA  02110-1301, USA.

package gofaxlib

import (
	"code.google.com/p/go-uuid/uuid"
	"errors"
	"fmt"
	"github.com/fiorix/go-eventsocket/eventsocket"
	"strconv"
	"strings"
	"time"
)

type Resolution struct {
	X uint
	Y uint
}

func (r Resolution) String() string {
	return fmt.Sprintf("%vx%v", r.X, r.Y)
}

func parseResolution(resstr string) (*Resolution, error) {
	parts := strings.SplitN(resstr, "x", 2)
	if len(parts) != 2 {
		return nil, errors.New("Error parsing resolution string")
	}
	res := new(Resolution)
	if x, err := strconv.ParseUint(parts[0], 10, 0); err == nil {
		res.X = uint(x)
	} else {
		return nil, err
	}
	if y, err := strconv.ParseUint(parts[1], 10, 0); err == nil {
		res.Y = uint(y)
	} else {
		return nil, err
	}
	return res, nil
}

type PageResult struct {
	Ts               time.Time
	Page             uint
	BadRows          uint
	LongestBadRowRun uint
	EncodingName     string
	ImagePixelSize   Resolution
	FilePixelSize    Resolution
	ImageResolution  Resolution
	FileResolution   Resolution
	ImageSize        uint
}

func (p PageResult) String() string {
	return fmt.Sprintf("Image Size: %v, Compression: %v, Comp Size: %v bytes, Bad Rows: %v",
		p.ImagePixelSize, p.EncodingName, p.ImageSize, p.BadRows)
}

type FaxResult struct {
	uuid       uuid.UUID
	sessionlog SessionLogger

	StartTs time.Time
	EndTs   time.Time

	Hangupcause string

	TotalPages       uint
	TransferredPages uint
	Ecm              bool
	RemoteID         string
	ResultCode       int // SpanDSP, not HylaFAX!
	ResultText       string
	Success          bool
	TransferRate     uint
	NegotiateCount   uint

	PageResults []PageResult
}

func NewFaxResult(uuid uuid.UUID, sessionlog SessionLogger) *FaxResult {
	f := &FaxResult{
		uuid:       uuid,
		sessionlog: sessionlog,
	}
	return f
}

func (f *FaxResult) AddEvent(ev *eventsocket.Event) {
	switch ev.Get("Event-Name") {
	case "CHANNEL_CALLSTATE":
		// Call state has changed
		callstate := ev.Get("Channel-Call-State")
		f.sessionlog.Log("Call state change:", callstate)
		if callstate == "ACTIVE" {
			f.StartTs = time.Now()
		}
		if callstate == "HANGUP" {
			f.EndTs = time.Now()
			f.Hangupcause = ev.Get("Hangup-Cause")
		}

	case "CUSTOM":
		// Fax results have changed
		action := ""
		switch ev.Get("Event-Subclass") {
		case "spandsp::rxfaxnegociateresult":
			fallthrough
		case "spandsp::txfaxnegociateresult":
			f.NegotiateCount++
			if ecm := ev.Get("Fax-Ecm-Used"); ecm == "on" {
				f.Ecm = true
			}
			f.RemoteID = ev.Get("Fax-Remote-Station-Id")
			if rate, err := strconv.ParseUint(ev.Get("Fax-Transfer-Rate"), 10, 0); err == nil {
				f.TransferRate = uint(rate)
			}
			f.sessionlog.Log(fmt.Sprintf("Remote ID: \"%v\", Transfer Rate: %v, ECM=%v", f.RemoteID, f.TransferRate, f.Ecm))

		case "spandsp::rxfaxpageresult":
			action = "received"
			fallthrough
		case "spandsp::txfaxpageresult":
			if action == "" {
				action = "sent"
			}
			// A page was transferred
			if pages, err := strconv.ParseUint(ev.Get("Fax-Document-Transferred-Pages"), 10, 0); err == nil {
				f.TransferredPages = uint(pages)
			}

			pr := new(PageResult)
			pr.Page = f.TransferredPages

			if badrows, err := strconv.ParseUint(ev.Get("Fax-Bad-Rows"), 10, 0); err == nil {
				pr.BadRows = uint(badrows)
			}
			pr.EncodingName = ev.Get("Fax-Encoding-Name")
			if imgsize, err := parseResolution(ev.Get("Fax-Image-Pixel-Size")); err == nil {
				pr.ImagePixelSize = *imgsize
			}
			if filesize, err := parseResolution(ev.Get("Fax-File-Image-Pixel-Size")); err == nil {
				pr.FilePixelSize = *filesize
			}
			if imgres, err := parseResolution(ev.Get("Fax-Image-Resolution")); err == nil {
				pr.ImageResolution = *imgres
			}
			if fileres, err := parseResolution(ev.Get("Fax-File-Image-Resolution")); err == nil {
				pr.FileResolution = *fileres
			}
			if size, err := strconv.ParseUint(ev.Get("Fax-Image-Size"), 10, 0); err == nil {
				pr.ImageSize = uint(size)
			}
			if badrowrun, err := strconv.ParseUint(ev.Get("Fax-Longest-Bad-Row-Run"), 10, 0); err == nil {
				pr.LongestBadRowRun = uint(badrowrun)
			}

			f.PageResults = append(f.PageResults, *pr)
			f.sessionlog.Log(fmt.Sprintf("Page %d %v: %v", f.TransferredPages, action, *pr))

		case "spandsp::rxfaxresult":
			fallthrough
		case "spandsp::txfaxresult":
			if totalpages, err := strconv.ParseUint(ev.Get("Fax-Document-Total-Pages"), 10, 0); err == nil {
				f.TotalPages = uint(totalpages)
			}
			if transferredpages, err := strconv.ParseUint(ev.Get("Fax-Document-Transferred-Pages"), 10, 0); err == nil {
				f.TransferredPages = uint(transferredpages)
			}
			if ecm := ev.Get("Fax-Ecm-Used"); ecm == "on" {
				f.Ecm = true
			}
			f.RemoteID = ev.Get("Fax-Remote-Station-Id")
			if resultcode, err := strconv.Atoi(ev.Get("Fax-Result-Code")); err == nil {
				f.ResultCode = resultcode
			}
			f.ResultText = ev.Get("Fax-Result-Text")
			if ev.Get("Fax-Success") == "1" {
				f.Success = true
			}
			if rate, err := strconv.ParseUint(ev.Get("Fax-Transfer-Rate"), 10, 0); err == nil {
				f.TransferRate = uint(rate)
			}

		}
	}

}
