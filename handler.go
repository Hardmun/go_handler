package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
)

type ipList [1]string

func (l *ipList) contains(ip string) bool {
	for _, i := range l {
		if i == ip {
			return true
		}
	}
	return false
}

type errResp struct {
	Error string `json:"error"`
}

func sendResponse(w *http.ResponseWriter, eR any) {
	err := json.NewEncoder(*w).Encode(eR)
	if err != nil {
		_, err = fmt.Fprint(*w, err.Error())
		fmt.Println(err.Error())
	}
}

func getRequestError(r *http.Request) (*errResp, error) {
	var (
		ip  string
		err error
	)

	var ips = ipList{"127.0.0.1"}
	eR := errResp{}

	ip, _, err = net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		eR.Error = err.Error()
		return &eR, nil
	} else if !ips.contains(ip) {
		eR.Error = fmt.Sprintf("IP is not allowed: %v", ip)
		return &eR, nil
	}
	if xri := r.Header.Get("X-Real-Ip"); len(xri) > 0 {
		if rIP := net.ParseIP(xri); rIP != nil {
			ip = rIP.String()
			if !ips.contains(ip) {
				eR.Error = fmt.Sprintf("IP is not allowed: %v", ip)
				return &eR, nil
			}
		}
	}

	return nil, nil
}

func requestHandler(w http.ResponseWriter, r *http.Request) {
	var (
		err error
		eR  *errResp
	)

	w.Header().Set("Content-Type", "application/json")

	if eR, err = getRequestError(r); err != nil {
		log.Println(err.Error())
	} else if eR != nil {
		sendResponse(&w, &eR)
	} else {
		//fmt.Fprint(w, "All good")
	}
}

func main() {
	http.HandleFunc("/okkam/geturl", requestHandler)
	err := http.ListenAndServe(":4545", nil)
	if err != nil {
		log.Fatal(err)
	}
}
