
// pacman-watch monitors Arch Linux updates for trouble
//   https://github.com/networkimprov/pacman-watch
//
// "service.go" HTTP server app
//
// Copyright 2014 by Liam Breck


package main

import (
  "os"
  "path/filepath"
  "io"
  "io/ioutil"
  "fmt"
  "net/http"
  "time"
)

var sDirname = filepath.Dir(os.Args[0])
var sTimer = map[string]*time.Timer{}
var sPassword []byte
//var sResponseTpl = `{"result":"%s", "message":"%s"}`

func main() {
  var err error
  sPassword, err = ioutil.ReadFile(sDirname+"/watch.conf")
  if err != nil { panic(err) }
  err = os.MkdirAll(sDirname+"/timer", os.FileMode(0755))
  if err != nil { panic(err) }

  http.HandleFunc("/", reqLog)
  http.HandleFunc("/open", reqOpen)
  http.HandleFunc("/close", reqClose)
  http.ListenAndServe(":4321", nil)
}

func reqLog(oResp http.ResponseWriter, iReq *http.Request) {
  fmt.Fprintf(oResp, "/open?client=xyz&pw=password\r\n/close?client=xyz&ticket=xyz-time&pw=password\r\n")
  return
  aF, err := os.Open(sDirname+"/watch.log")
  if err != nil { panic(err) }
  _, err = io.Copy(oResp, aF)
  if err != nil { panic(err) }
}

func reqOpen(oResp http.ResponseWriter, iReq *http.Request) {
  aV := iReq.URL.Query()
  if (aV.Get("pw") != string(sPassword)) {
    fmt.Fprintf(oResp, "error\r\ninvalid password\r\n")
    return
  }
  aClient := aV.Get("client")
  if sTimer[aClient] != nil {
    fmt.Fprintf(oResp, "error\r\nexpecting close\r\n")
    return
  }
  aTime, err := time.Now().MarshalText()
  if err != nil { panic(err) }
  aTicket := aClient+"-"+string(aTime)
  err = ioutil.WriteFile(sDirname+"/timer/"+aClient, aTime, os.FileMode(0644)) // os.Sync
  sTimer[aClient] = time.AfterFunc(time.Duration(10)*time.Minute, func() { timeUp(aClient) })
  if err != nil { panic(err) }
  fmt.Fprintf(oResp, "ok\r\n%s\r\n", aTicket)
}

func timeUp(iClient string) {
  sTimer[iClient] = nil
  err := os.Remove(sDirname+"/timer/"+iClient) // os.Sync
  if err != nil { panic(err) }
  fmt.Println("time to email for help!")

}

func reqClose(oResp http.ResponseWriter, iReq *http.Request) {
  aV := iReq.URL.Query()
  if (aV.Get("pw") != string(sPassword)) {
    fmt.Fprintf(oResp, "error\r\ninvalid password\r\n")
    return
  }
  aClient := aV.Get("client")
  //aTicket := aV.Get("ticket")
  if sTimer[aClient] == nil {
    fmt.Fprintf(oResp, "error\r\nalready closed\r\n")
    return
  }
  sTimer[aClient].Stop()
  sTimer[aClient] = nil
  err := os.Remove(sDirname+"/timer/"+aClient) // os.Sync
  if err != nil { panic(err) }
  fmt.Fprintf(oResp, "ok\r\n")
}

