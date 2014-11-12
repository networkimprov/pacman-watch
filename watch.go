
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
  "sync"
  "time"
)


var sDirname = filepath.Dir(os.Args[0])
var sLog *os.File
type tClient struct {
  timer *time.Timer
  retry *bool
  open time.Time
  flag bool
  updt bool
  timeup chan byte
}
var sClient = map[string]*tClient{}
var sStatusClient = &tClient{flag: true}
var sStatus sync.Mutex
var sConfig = struct {
  Http string
  Password string
  OkWait, UpdateWait int
  To, From string
  test bool
  okD, updateD time.Duration
}{test : len(os.Args) > 1 && os.Args[1] == "test"}
//var sResponseTpl = `{"error":%d, "message":"%s"}`
const kEmailTmpl = "To: %s\r\nFrom: %s\r\nSubject: pacman-watch %s\r\nDate: %s\r\n\r\n%s"


func main() {
  var err error
  err = os.MkdirAll(sDirname+"/watch.d", os.FileMode(0755))
  if err != nil { panic(err) }
  sLog, err = os.OpenFile(sDirname+"/watch.log", os.O_CREATE|os.O_APPEND|os.O_WRONLY, os.FileMode(0644))
  if err != nil { panic(err) }
  aConfig, err := ioutil.ReadFile(sDirname+"/watch.conf")
  if err != nil { panic(err) }
  err = json.Unmarshal(aConfig, &sConfig)
  if err != nil { panic(err) }
  sConfig.okD = time.Duration(sConfig.OkWait)*time.Minute
  sConfig.updateD = time.Duration(sConfig.UpdateWait)*time.Second

  if ! sConfig.test {
    if err = sendMail("status", "server started", nil); err != nil {
      fmt.Printf("email error: %s\n", err.Error())
    }
  }
  _, err = fmt.Fprintf(sLog, "RESUME %s\n", time.Now().Format(time.RFC3339))
  if err != nil { panic(err) }

  aDir, err := ioutil.ReadDir(sDirname+"/watch.d")
  if err != nil { panic(err) }
  for a := range aDir {
    aClient := aDir[a].Name()
    aData, err := ioutil.ReadFile(sDirname+"/watch.d/"+aClient)
    if err != nil { panic(err) }
    aPair := strings.Split(string(aData), " ")
    aOpen, err := time.Parse(time.RFC3339, aPair[0])
    if err != nil { panic(err) }
    aNew := &tClient{open: aOpen, retry: new(bool), flag: aPair[1] != "ok", updt: aPair[1] == "update", timeup: make(chan byte, 1)}
    *aNew.retry = true
    aWait := sConfig.okD; if aNew.flag { aWait = sConfig.updateD }
    aNew.timer = time.AfterFunc(aWait - time.Since(aOpen), func() { timeUp(aClient, aNew) })
    sClient[aClient] = aNew
    updateStatus(aNew)
  }

  http.HandleFunc("/", reqLog)
  http.HandleFunc("/ping", reqPing)
  http.HandleFunc("/status", reqStatus)
  http.ListenAndServe(sConfig.Http, nil)
}

func reqLog(oResp http.ResponseWriter, iReq *http.Request) {
  fmt.Fprintf(oResp, "/ping?client=xyz&pw=password&status={ok,update}\r\n/status\r\n\r\nstatus now %v\r\n\r\n", !sStatusClient.flag)
  aF, err := os.Open(sDirname+"/watch.log")
  if err != nil { panic(err) }
  defer aF.Close()
  _, err = io.Copy(oResp, aF)
  if err != nil { panic(err) }
}

func reqStatus(oResp http.ResponseWriter, iReq *http.Request) {
  aS := "ok"; if sStatusClient.flag { aS = "error" }
  fmt.Fprintf(oResp, "%s\r\n", aS)
}

func updateStatus(iObj *tClient) {
  sStatus.Lock()
  if iObj != sStatusClient && (iObj.updt || !sStatusClient.updt) && iObj.open.After(sStatusClient.open) {
    sStatusClient = iObj
  }
  sStatus.Unlock()
}

func reqPing(oResp http.ResponseWriter, iReq *http.Request) {
  aV := iReq.URL.Query()
  if (aV.Get("pw") != sConfig.Password) {
    fmt.Fprintf(oResp, "error\r\nmissing or invalid password\r\n")
    return
  }
  aStatus := aV.Get("status")
  if aStatus != "ok" && aStatus != "update" {
    fmt.Fprintf(oResp, "error\r\nmissing or invalid status\r\n")
    return
  }
  aClient := aV.Get("client")
  aObj := sClient[aClient]
  aWait := sConfig.updateD; if aStatus == "ok" { aWait = sConfig.okD }
  if aObj == nil {
    aObj = &tClient{timeup: make(chan byte, 1)}
    aObj.timer = time.AfterFunc(aWait, func() { timeUp(aClient, aObj) })
    sClient[aClient] = aObj
  } else if ! aObj.timer.Reset(aWait) {
    <-aObj.timeup
    *aObj.retry = false
    if aStatus == "ok" && ! sConfig.test {
      go sendMail("status", aClient+" failure has been resolved", nil)
    }
  }
  aRetry := true
  aObj.retry = &aRetry
  aObj.open = time.Now()
  aObj.flag = aStatus != "ok"
  aObj.updt = aStatus == "update"
  updateStatus(aObj)
  aTime := aObj.open.Format(time.RFC3339)
  err := WriteSync(sDirname+"/watch.d/"+aClient, []byte(aTime+" "+aStatus), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(0644))
  if err != nil { panic(err) }
  fmt.Fprintf(oResp, "ok\r\n")
}

func timeUp(iClient string, iObj *tClient) {
  fmt.Println("time to email for help!")
  aReason := "timeup"; if iObj.flag { aReason = "UPDATE" }
  aMsg := " has not been heard from in too long"; if iObj.flag { aMsg = " failed to complete an update" }
  iObj.flag = true
  _, err := fmt.Fprintf(sLog, "%s %s, %s %.1fm\n", aReason, iClient, time.Now().Format(time.RFC3339), time.Since(iObj.open).Minutes())
  if err != nil { panic(err) }
  iObj.timeup <- 0
  sendMail("alert", iClient+aMsg, iObj.retry)
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


