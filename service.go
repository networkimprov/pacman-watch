
// pacman-watch monitors Arch Linux updates for trouble
//   https://github.com/networkimprov/pacman-watch
//
// "service.go" HTTP server app
//
// Copyright 2014 by Liam Breck


package main

import (
  "path/filepath"
  "fmt"
  "net/http"
  "io"
  "io/ioutil"
  "encoding/json"
  "net"
  "os"
  "net/smtp"
  "strings"
  "time"
)


var sDirname = filepath.Dir(os.Args[0])
var sTimer = map[string]*time.Timer{}
var sTimeUp = time.Timer{}
var sConfig = struct {
  Http string
  Password string
  Wait int
  To []string
  From string
  Message string
}{}
//var sResponseTpl = `{"error":%d, "message":"%s"}`

func main() {
  var err error
  aConfig, err := ioutil.ReadFile(sDirname+"/watch.conf")
  if err != nil { panic(err) }
  err = json.Unmarshal(aConfig, &sConfig)
  if err != nil { panic(err) }
  err = os.MkdirAll(sDirname+"/timer", os.FileMode(0755))
  if err != nil { panic(err) }

  if sConfig.Password != "password" {
    for a := range sConfig.To {
      sendMail(sConfig.To[a], "server started")
    }
  }

  http.HandleFunc("/", reqLog)
  http.HandleFunc("/open", reqOpen)
  http.HandleFunc("/close", reqClose)
  http.ListenAndServe(sConfig.Http, nil)
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
  if (aV.Get("pw") != sConfig.Password) {
    fmt.Fprintf(oResp, "error\r\ninvalid password\r\n")
    return
  }
  aClient := aV.Get("client")
  if sTimer[aClient] != nil {
    fmt.Fprintf(oResp, "error\r\nalready opened\r\n")
    return
  }
  aTime := time.Now().Format(time.RFC3339)
  err := ioutil.WriteFile(sDirname+"/timer/"+aClient, []byte(aTime), os.FileMode(0644)) // os.Sync
  if err != nil { panic(err) }
  sTimer[aClient] = time.AfterFunc(time.Duration(sConfig.Wait)*time.Second, func() { timeUp(aClient) })
  fmt.Fprintf(oResp, "ok\r\n%s-%s\r\n", aClient, aTime)
}

func timeUp(iClient string) {
  sTimer[iClient] = &sTimeUp
  fmt.Println("time to email for help!")
  for a := range sConfig.To {
    sendMail(sConfig.To[a], iClient+" failed to complete an update")
  }
  err := AppendFile(sDirname+"/timer/"+iClient, []byte(" timeup")) // os.Sync
  if err != nil { panic(err) }
}

func sendMail(iTo, iMsg string) {
  var err error
  aMx, err := net.LookupMX(strings.Split(iTo, "@")[1])
  if err != nil { panic(err) }
  aConn, err := smtp.Dial(aMx[0].Host+":25")
  if err != nil { panic(err) }
  err = aConn.Hello(strings.Split(sConfig.From, "@")[1])
  if err != nil { panic(err) }
  err = aConn.Mail(sConfig.From)
  if err != nil { panic(err) }
  err = aConn.Rcpt(iTo)
  if err != nil { panic(err) }
  aW, err := aConn.Data()
  if err != nil { panic(err) }
  fmt.Fprintf(aW, sConfig.Message, iTo, sConfig.From, time.Now().Format(time.RFC822Z), iMsg)
  err = aW.Close()
  if err != nil { panic(err) }
  err = aConn.Quit()
  if err != nil { panic(err) }
}

func reqClose(oResp http.ResponseWriter, iReq *http.Request) {
  aV := iReq.URL.Query()
  if (aV.Get("pw") != sConfig.Password) {
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

func AppendFile(iName string, iData []byte) error {
  aF, err := os.OpenFile(iName, os.O_APPEND|os.O_WRONLY, 0)
  if err != nil { return err }
  defer aF.Close()
  _, err = aF.Write(iData)
  if err != nil { return err }
  return nil
}


