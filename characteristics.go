package hap

import (
	"github.com/brutella/hap/accessory"
	"github.com/brutella/hap/characteristic"
	"github.com/brutella/hap/log"
	"github.com/xiam/to"

	"encoding/json"
	"net/http"
	"strings"
)

type CharacteristicData struct {
	Aid   uint64      `json:"aid"`
	Iid   uint64      `json:"iid"`
	Value interface{} `json:"value,omitempty"`

	// optional values
	Type        *string     `json:"type,omitempty"`
	Permissions []string    `json:"perms,omitempty"`
	Status      *int        `json:"status,omitempty"`
	Events      *bool       `json:"ev,omitempty"`
	Format      *string     `json:"format,omitempty"`
	Unit        *string     `json:"unit,omitempty"`
	MinValue    interface{} `json:"minValue,omitempty"`
	MaxValue    interface{} `json:"maxValue,omitempty"`
	MinStep     interface{} `json:"minStep,omitempty"`
	MaxLen      *int        `json:"maxLen,omitempty"`

	Remote   *bool `json:"remote,omitempty"`
	Response *bool `json:"r,omitempty"`
}

func (srv *Server) GetCharacteristics(res http.ResponseWriter, req *http.Request) {
	if !srv.isPaired() {
		log.Info.Println("not paired")
		jsonError(res, JsonStatusInsufficientPrivileges)
		return
	}

	// id=1.4,1.5
	v := req.FormValue("id")
	if len(v) == 0 {
		jsonError(res, JsonStatusInvalidValueInRequest)
		return
	}

	meta := req.FormValue("meta") == "1"
	perms := req.FormValue("perms") == "1"
	typ := req.FormValue("type") == "1"
	ev := req.FormValue("ev") == "1"

	arr := []*CharacteristicData{}
	err := false
	for _, str := range strings.Split(v, ",") {
		ids := strings.Split(str, ".")
		if len(ids) != 2 {
			continue
		}
		cdata := &CharacteristicData{
			Aid: to.Uint64(ids[0]),
			Iid: to.Uint64(ids[1]),
		}
		arr = append(arr, cdata)

		c := srv.findC(cdata.Aid, cdata.Iid)
		if c == nil {
			err = true
			status := JsonStatusServiceCommunicationFailure
			cdata.Status = &status
			continue
		}

		cdata.Value = c.ValueRequest(req)

		if meta {
			cdata.Format = &c.Format
			cdata.Unit = &c.Unit
			if c.MinVal != nil {
				cdata.MinValue = c.MinVal
			}
			if c.MaxVal != nil {
				cdata.MaxValue = c.MaxVal
			}
			if c.StepVal != nil {
				cdata.MinStep = c.StepVal
			}

			if c.MaxLen > 0 {
				cdata.MaxLen = &c.MaxLen
			}
		}

		// Should the response include the events flag?
		if ev {
			var ev bool
			if v, ok := c.Events[req.RemoteAddr]; ok {
				ev = v
			}
			cdata.Events = &ev
		}

		if perms {
			cdata.Permissions = c.Permissions
		}

		if typ {
			cdata.Type = &c.Type
		}
	}

	resp := struct {
		Characteristics []*CharacteristicData `json:"characteristics"`
	}{arr}

	log.Debug.Println(toJSON(resp))

	if err {
		jsonMultiStatus(res, resp)
	} else {
		jsonOK(res, resp)
	}
}

func (srv *Server) PutCharacteristics(res http.ResponseWriter, req *http.Request) {
	if !srv.isPaired() {
		log.Info.Println("not paired")
		jsonError(res, JsonStatusInsufficientPrivileges)
		return
	}

	data := struct {
		Cs []CharacteristicData `json:"characteristics"`
	}{}
	err := json.NewDecoder(req.Body).Decode(&data)
	if err != nil {
		jsonError(res, JsonStatusInvalidValueInRequest)
		return
	}

	log.Debug.Println(toJSON(data))

	arr := []*CharacteristicData{}
	for _, d := range data.Cs {
		c := srv.findC(d.Aid, d.Iid)
		cdata := &CharacteristicData{
			Aid: d.Aid,
			Iid: d.Iid,
		}

		if c == nil {
			status := JsonStatusServiceCommunicationFailure
			cdata.Status = &status
			arr = append(arr, cdata)
			continue
		}

		if d.Response != nil {
			cdata.Value = c.ValueRequest(req)
			arr = append(arr, cdata)
		}

		if d.Value != nil {
			c.SetValueRequest(d.Value, req)
		}

		if d.Events != nil {
			if !c.IsObservable() {
				status := JsonStatusNotificationNotSupported
				cdata.Status = &status
				arr = append(arr, cdata)
			} else {
				c.Events[req.RemoteAddr] = *d.Events
			}
		}
	}

	if len(arr) == 0 {
		res.WriteHeader(http.StatusNoContent)
		return
	}

	resp := struct {
		Characteristics []*CharacteristicData `json:"characteristics"`
	}{arr}

	log.Debug.Println(toJSON(resp))
	jsonMultiStatus(res, resp)
}

func (srv *Server) findC(aid, iid uint64) *characteristic.C {
	var as []*accessory.A
	as = append(as, srv.a)
	as = append(as, srv.as[:]...)

	for _, a := range as {
		if a.Id == aid {
			for _, s := range a.Ss {
				for _, c := range s.Cs {
					if c.Id == iid {
						return c
					}
				}
			}
		}
	}

	return nil
}