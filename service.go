
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
var sLog *os.File
type tClient struct {
  timer *time.Timer
  retry *bool
}
var sClient = map[string]*tClient{}
var sConfig = struct {
  Http string
  Password string
  Wait int
  To, From string
  test bool
}{test : len(os.Args) > 1 && os.Args[1] == "test"}
//var sResponseTpl = `{"error":%d, "message":"%s"}`
const kEmailTmpl = "To: %s\r\nFrom: %s\r\nSubject: pacman-watch %s\r\nDate: %s\r\n\r\n%s"

func main() {
  var err error
  err = os.MkdirAll(sDirname+"/timer", os.FileMode(0755))
  if err != nil { panic(err) }
  sLog, err = os.OpenFile(sDirname+"/watch.log", os.O_CREATE|os.O_APPEND|os.O_WRONLY, os.FileMode(0644))
  if err != nil { panic(err) }
  aConfig, err := ioutil.ReadFile(sDirname+"/watch.conf")
  if err != nil { panic(err) }
  err = json.Unmarshal(aConfig, &sConfig)
  if err != nil { panic(err) }

  if ! sConfig.test {
    if err = sendMail("status", "server started", nil); err != nil {
      fmt.Printf("email error: %s\n", err.Error())
    }
  }
  _, err = fmt.Fprintf(sLog, "RESUME %s\n", time.Now().Format(time.RFC3339))
  if err != nil { panic(err) }

  aPend, err := ioutil.ReadDir(sDirname+"/timer")
  if err != nil { panic(err) }
  for a := range aPend {
    aClient := aPend[a].Name()
    aData, err := ioutil.ReadFile(sDirname+"/timer/"+aClient)
    if err != nil { panic(err) }
    if len(aData) == 0 {
      continue
    }
    aOpen, err := time.Parse(time.RFC3339, string(aData))
    if err != nil { panic(err) }
    aWait := time.Duration(sConfig.Wait)*time.Second - time.Since(aOpen)
    aRetry := true
    sClient[aClient] = &tClient{
      retry: &aRetry,
      timer: time.AfterFunc(aWait, func() { timeUp(aClient, &aRetry) }),
    }
  }

  http.HandleFunc("/", reqLog)
  http.HandleFunc("/open", reqOpen)
  http.HandleFunc("/close", reqClose)
  http.ListenAndServe(sConfig.Http, nil)
}

func reqLog(oResp http.ResponseWriter, iReq *http.Request) {
  fmt.Fprintf(oResp, "/open?client=xyz&pw=password\r\n/close?client=xyz&pw=password\r\n\r\n")
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
  if sClient[aClient] != nil {
    fmt.Fprintf(oResp, "error\r\nalready opened\r\n")
    return
  }
  aTime := time.Now().Format(time.RFC3339)
  err := WriteSync(sDirname+"/timer/"+aClient, []byte(aTime), os.O_CREATE|os.O_WRONLY, os.FileMode(0644))
  if err != nil { panic(err) }
  aRetry := true
  sClient[aClient] = &tClient{
    retry: &aRetry,
    timer: time.AfterFunc(time.Duration(sConfig.Wait)*time.Second, func() { timeUp(aClient, &aRetry) }),
  }
  fmt.Fprintf(oResp, "ok\r\n")
}

func timeUp(iClient string, iRetry *bool) {
  fmt.Println("time to email for help!")
  var err error
  _, err = fmt.Fprintf(sLog, "TIMEUP %s, %s %.1fm\n", iClient, time.Now().Format(time.RFC3339), (time.Duration(sConfig.Wait)*time.Second).Minutes())
  if err != nil { panic(err) }
  sendMail("alert", iClient+" failed to complete an update", iRetry)
}

func sendMail(iSubject, iMsg string, iRetry *bool) error {
  for {
    var err error
    var aMx []*net.MX
    var aConn *smtp.Client
    var aW io.WriteCloser
    aMx, err = net.LookupMX(strings.Split(sConfig.To, "@")[1])
    if err == nil {
      aConn, err = smtp.Dial(aMx[0].Host+":25")
    }
    if err == nil {
      err = aConn.Hello(strings.Split(sConfig.From, "@")[1])
      if err == nil {
        err = aConn.Mail(sConfig.From)
      }
      if err == nil {
        err = aConn.Rcpt(sConfig.To)
      }
      if err == nil {
        aW, err = aConn.Data()
      }
      if err == nil {
        _, err = fmt.Fprintf(aW, kEmailTmpl, sConfig.To, sConfig.From, iSubject, time.Now().Format(time.RFC822Z), iMsg)
        if err1 := aW.Close(); err == nil { err = err1 }
      }
      if err == nil {
        err = aConn.Quit()
      }
      if err == nil {
        return nil
      }
      aConn.Close()
    }
    if iRetry == nil || *iRetry == false {
      return err
    }
    time.Sleep(time.Duration(1)*time.Minute)
  }
}

func reqClose(oResp http.ResponseWriter, iReq *http.Request) {
  aV := iReq.URL.Query()
  if (aV.Get("pw") != sConfig.Password) {
    fmt.Fprintf(oResp, "error\r\ninvalid password\r\n")
    return
  }
  aClient := aV.Get("client")
  if sClient[aClient] == nil {
    fmt.Fprintf(oResp, "error\r\nalready closed\r\n")
    return
  }
  if ! sClient[aClient].timer.Stop() && ! sConfig.test {
    go sendMail("status", aClient+" failure has been resolved", nil)
  }
  *sClient[aClient].retry = false
  sClient[aClient] = nil
  var err error
  aData, err := ioutil.ReadFile(sDirname+"/timer/"+aClient)
  if err != nil { panic(err) }
  aStart, err := time.Parse(time.RFC3339, string(aData))
  if err != nil { panic(err) }
  _, err = fmt.Fprintf(sLog, "closed %s, %s %.1fm\n", aClient, aData, time.Since(aStart).Minutes())
  if err != nil { panic(err) }
  err = WriteSync(sDirname+"/timer/"+aClient, []byte{}, os.O_TRUNC|os.O_WRONLY, 0)
  if err != nil { panic(err) }
  fmt.Fprintf(oResp, "ok\r\n")
}

func WriteSync(iName string, iData []byte, iFlag int, iMode os.FileMode) error {
  aF, err := os.OpenFile(iName, iFlag, iMode)
  if err != nil { return err }
  defer aF.Close()
  _, err = aF.Write(iData)
  if err != nil { return err }
  err = aF.Sync()
  if err != nil { return err }
  return nil
}


