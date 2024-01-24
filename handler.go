package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/time/rate"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const rateLimit = 20

var ipLimitGLB = ipLimiter{
	limiter: make(map[string]*rate.Limiter),
}

var (
	absPath  string
	logFile  *os.File
	settings settingsType
	mu       sync.RWMutex
)

type settingsType struct {
	Dir string `json:"dir"`
	Ip  ipList `json:"ip"`
	Url string `json:"url"`
}

func (s *settingsType) getFileDir() string {
	mu.RLock()
	defer mu.RUnlock()

	return s.Dir
}

func (s *settingsType) getIPs() ipList {
	mu.RLock()
	defer mu.RUnlock()

	return s.Ip
}

func (s *settingsType) getURL() string {
	mu.RLock()
	defer mu.RUnlock()

	return s.Url
}

type ipLimiter struct {
	limiter map[string]*rate.Limiter
	mu      sync.Mutex
}

func (ipl *ipLimiter) getLimiter(ip string) *rate.Limiter {
	ipl.mu.Lock()
	defer ipl.mu.Unlock()

	lim, ok := ipl.limiter[ip]
	if !ok {
		lim = rate.NewLimiter(rateLimit, 1)
		ipl.limiter[ip] = lim
	}

	return lim
}

type ipList []string

func (l *ipList) contains(ip string) bool {
	for _, i := range *l {
		if i == ip {
			return true
		}
	}
	return false
}

type errResp struct {
	Error string `json:"error"`
}

type jsonResponse struct {
	Url string `json:"url"`
}

func readSettings() error {
	var (
		file     *os.File
		jsonData []byte
		err      error
	)
	jsonFile := filepath.Join(absPath, "settings.json")
	if l, errInfo := os.Stat(jsonFile); !(errInfo == nil && !l.IsDir()) {
		settings.Dir = "C:/ordFiles"
		settings.Ip = make(ipList, 0)
		settings.Url = "http://127.0.0.1/okkam/files"

		jsonData, err = json.MarshalIndent(settings, "", "  ")
		if err != nil {
			return err
		}

		file, err = os.Create(jsonFile)
		if err != nil {
			return err
		}
		defer func() {
			err = file.Close()
			if err != nil {
				log.Fatal(err)
			}
		}()
		_, err = file.Write(jsonData)
		if errInfo != nil {
			return err
		}
	} else {
		jsonData, err = os.ReadFile(jsonFile)
		if errInfo != nil {
			return err
		}

		err = json.Unmarshal(jsonData, &settings)
		if errInfo != nil {
			return err
		}
	}
	return nil
}

func loggMessage(err any) {
	var (
		lgType string
		msg    string
	)

	switch err.(type) {
	case *error:
		lgType = "[error]"
		fullErr := *err.(*error)
		msg = fullErr.Error()
	case string:
		lgType = "[info]"
		msg = err.(string)
	}
	errorLog := log.New(logFile, lgType, log.LstdFlags|log.Lshortfile)
	errorLog.Println(msg)
}

func openLogFile(path string) (*os.File, error) {
	logDir := filepath.Join(absPath, "logs")
	if l, err := os.Stat(logDir); !(err == nil && l.IsDir()) {
		err = os.Mkdir(logDir, 777)
		if err != nil {
			log.Fatal(err)
		}
	}
	lFile, err := os.OpenFile(filepath.Join(logDir, path), os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0644)
	if err != nil {
		return nil, err
	}
	return lFile, nil
}

func sendResponse(w *http.ResponseWriter, jsonData any) {
	err := json.NewEncoder(*w).Encode(jsonData)
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
	eR := errResp{}

	if r.Method != "POST" {
		eR.Error = "Only POST allowed"
		return &eR, nil
	}

	ips := settings.getIPs()

	ip, _, err = net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		eR.Error = err.Error()
		return &eR, nil
	} else if len(ips) > 0 && !ips.contains(ip) {
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

	limiter := ipLimitGLB.getLimiter(ip)
	if !limiter.Allow() {
		eR.Error = fmt.Sprintf("Rate limit exceeded for IP: %v", ip)
		return &eR, nil
	}

	return nil, nil
}

func readRequest(w *http.ResponseWriter, r *http.Request) error {
	var (
		body []byte
		err  error
		file *os.File
	)

	body, err = io.ReadAll(r.Body)
	if err != nil {
		return err
	}

	filePth := settings.getFileDir()

	if dirList, ok := r.Header["Dir"]; ok && len(dirList) > 0 {
		dirPath := filepath.Join(filePth, dirList[0])
		if l, errDir := os.Stat(dirPath); !(errDir == nil && l.IsDir()) {
			errDir = os.MkdirAll(dirPath, 777)
			if errDir != nil {
				return errDir
			}
		}

		if fileName, okFile := r.Header["Filename"]; okFile && len(fileName) > 0 {
			allowedFileName := url.PathEscape(fileName[0])
			filePath := filepath.Join(filePth, dirList[0], allowedFileName)
			file, err = os.Create(filePath)
			if err != nil {
				return err
			}
			defer func() {
				err = file.Close()
				if err != nil {
					loggMessage(&err)
				}
			}()
			_, err = file.Write(body)
			if err != nil {
				return err
			}
			jsonData := jsonResponse{Url: fmt.Sprint(settings.getURL(), "/", dirList[0], "/", allowedFileName)}
			sendResponse(w, &jsonData)
		}

	} else {
		return errors.New("expected Dir header")
	}

	return nil
}

func requestHandler(w http.ResponseWriter, r *http.Request) {
	var (
		err error
		eR  *errResp
	)

	w.Header().Set("Content-Type", "application/json")

	if eR, err = getRequestError(r); err != nil {
		loggMessage(&err)
	} else if eR != nil {
		sendResponse(&w, &eR)
		err = errors.New(eR.Error)
		loggMessage(&err)
	} else {
		err = readRequest(&w, r)
		if err != nil {
			loggMessage(&err)
			eR = new(errResp)
			eR.Error = err.Error()
			sendResponse(&w, &eR)
			if _, err = fmt.Fprint(w, err.Error()); err != nil {
				loggMessage(&err)
			}
		}
	}
}

func requestHandlerOpen(w http.ResponseWriter, r *http.Request) {
	rURL := strings.Replace(r.RequestURI, "/okkam/files/", "", -1)
	filePath := filepath.Join(settings.getFileDir(), rURL)
	//w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filepath.Base(filePath)))
	http.ServeFile(w, r, filePath)
}

func main() {
	var err error

	//exePath, errExe := os.Executable()
	//if errExe != nil {
	//	log.Fatal(errExe)
	//}
	//absPath = filepath.Dir(exePath)

	logFile, err = openLogFile("log.log")

	if err != nil {
		log.Fatal(err)
	}

	err = readSettings()
	if err != nil {
		log.Fatal()
	}

	//Closing the logFile and Exit
	defer func(logFile *os.File) {
		err = logFile.Close()
		if err != nil {
			log.Fatal(err)
		}
		os.Exit(0)
	}(logFile)

	http.HandleFunc("/okkam/api/v1/sendfile", requestHandler)
	http.Handle("/okkam/files/", http.StripPrefix("/okkam/files/", http.HandlerFunc(requestHandlerOpen)))

	//http.Handle("/okkam/files/", http.StripPrefix("/okkam/files/",
	//	http.FileServer(http.Dir(settings.getFileDir()))))

	err = http.ListenAndServe(":4545", nil)
	if err != nil {
		loggMessage(&err)
		log.Fatal(err)
	}
}
