
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
var sTimer = map[string]*time.Timer{}
var sConfig = struct {
  Http string
  Password string
  Wait int
  To, From, Message string
  test bool
}{test : len(os.Args) > 1 && os.Args[1] == "test"}
//var sResponseTpl = `{"error":%d, "message":"%s"}`

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
    if err = sendMail("server started", "once"); err != nil {
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
    aTime := strings.Split(string(aData), " ")
    if len(aTime) > 1 && aTime[1] == "timeup" {
      sTimer[aClient] = new(time.Timer)
      continue
    }
    aOpen, err := time.Parse(time.RFC3339, aTime[0])
    if err != nil { panic(err) }
    aWait := time.Duration(sConfig.Wait)*time.Second - time.Since(aOpen)
    sTimer[aClient] = time.AfterFunc(aWait, func() { timeUp(aClient) })
  }

  http.HandleFunc("/", reqLog)
  http.HandleFunc("/open", reqOpen)
  http.HandleFunc("/close", reqClose)
  http.ListenAndServe(sConfig.Http, nil)
}

func reqLog(oResp http.ResponseWriter, iReq *http.Request) {
  fmt.Fprintf(oResp, "/open?client=xyz&pw=password\r\n/close?client=xyz&ticket=xyz@time&pw=password\r\n\r\n")
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
  err := WriteSync(sDirname+"/timer/"+aClient, []byte(aTime), os.O_CREATE|os.O_WRONLY, os.FileMode(0644))
  if err != nil { panic(err) }
  sTimer[aClient] = time.AfterFunc(time.Duration(sConfig.Wait)*time.Second, func() { timeUp(aClient) })
  fmt.Fprintf(oResp, "ok\r\n%s@%s\r\n", aClient, aTime)
}

func timeUp(iClient string) {
  fmt.Println("time to email for help!")
  var err error
  _, err = fmt.Fprintf(sLog, "TIMEUP %s, %s %.1fm\n", iClient, time.Now().Format(time.RFC3339), (time.Duration(sConfig.Wait)*time.Second).Minutes())
  if err != nil { panic(err) }
  sendMail(iClient+" failed to complete an update", "retry")
  err = WriteSync(sDirname+"/timer/"+iClient, []byte(" timeup"), os.O_APPEND|os.O_WRONLY, 0)
  if err != nil { panic(err) }
}

func sendMail(iMsg, iRetry string) error {
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
        _, err = fmt.Fprintf(aW, sConfig.Message, sConfig.To, sConfig.From, time.Now().Format(time.RFC822Z), iMsg)
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
    if iRetry != "retry" {
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
  if sTimer[aClient] == nil {
    fmt.Fprintf(oResp, "error\r\nalready closed\r\n")
    return
  }
  if ! sTimer[aClient].Stop() && ! sConfig.test {
    go sendMail(aClient+" failure has been resolved", "retry")
  }
  sTimer[aClient] = nil
  var err error
  aTicket := strings.Split(aV.Get("ticket"), "@")
  aStart, err := time.Parse(time.RFC3339, aTicket[1])
  if err != nil { panic(err) }
  _, err = fmt.Fprintf(sLog, "closed %s, %s %.1fm\n", aTicket[0], aTicket[1], time.Since(aStart).Minutes())
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


