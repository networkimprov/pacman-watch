
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
  retry bool
  open time.Time
  timeup bool
}
var sClient = map[string]*tClient{}
var sStatusClient = &tClient{timeup: true}
var sStatus sync.Mutex
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
    aPair := strings.Split(string(aData), " ")
    aOpen, err := time.Parse(time.RFC3339, aPair[0])
    if err != nil { panic(err) }
    if aPair[1] == "closed" {
      updateStatus(&tClient{open: aOpen, timeup: false})
    } else {
      aNew := &tClient{open: aOpen, retry: true}
      sClient[aClient] = aNew
      aNew.timer = time.AfterFunc(time.Duration(sConfig.Wait)*time.Second - time.Since(aOpen), func() { timeUp(aClient, aNew) })
    }
  }

  http.HandleFunc("/", reqLog)
  http.HandleFunc("/open", reqOpen)
  http.HandleFunc("/close", reqClose)
  http.HandleFunc("/status", reqStatus)
  http.ListenAndServe(sConfig.Http, nil)
}

func reqLog(oResp http.ResponseWriter, iReq *http.Request) {
  fmt.Fprintf(oResp, "/open?client=xyz&pw=password\r\n/close?client=xyz&pw=password\r\n/status\r\n\r\n")
  aF, err := os.Open(sDirname+"/watch.log")
  if err != nil { panic(err) }
  defer aF.Close()
  _, err = io.Copy(oResp, aF)
  if err != nil { panic(err) }
}

func reqStatus(oResp http.ResponseWriter, iReq *http.Request) {
  aS := "ok"
  if sStatusClient.timeup {
    aS = "error"
  }
  fmt.Fprintf(oResp, "%s\r\n", aS)
}

func updateStatus(iObj *tClient) {
  sStatus.Lock()
  if iObj != sStatusClient && iObj.open.After(sStatusClient.open) {
    sStatusClient = iObj
  }
  sStatus.Unlock()
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
  aTime := time.Now().Format(time.RFC3339+" ")
  err := WriteSync(sDirname+"/timer/"+aClient, []byte(aTime), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(0644))
  if err != nil { panic(err) }
  aNew := &tClient{open: time.Now(), retry: true}
  aNew.timer = time.AfterFunc(time.Duration(sConfig.Wait)*time.Second, func() { timeUp(aClient, aNew) })
  sClient[aClient] = aNew
  fmt.Fprintf(oResp, "ok\r\n")
}

func timeUp(iClient string, iObj *tClient) {
  fmt.Println("time to email for help!")
  iObj.timeup = true
  updateStatus(iObj)
  var err error
  _, err = fmt.Fprintf(sLog, "TIMEUP %s, %s %.1fm\n", iClient, time.Now().Format(time.RFC3339), (time.Duration(sConfig.Wait)*time.Second).Minutes())
  if err != nil { panic(err) }
  sendMail("alert", iClient+" failed to complete an update", &iObj.retry)
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
  aObj := sClient[aClient]
  if ! aObj.timer.Stop() {
    aObj.retry = false
    if ! sConfig.test {
      go sendMail("status", aClient+" failure has been resolved", nil)
    }
  }
  aObj.timeup = false
  updateStatus(aObj)
  sClient[aClient] = nil
  var err error
  aData, err := ioutil.ReadFile(sDirname+"/timer/"+aClient)
  if err != nil { panic(err) }
  aStart, err := time.Parse(time.RFC3339+" ", string(aData))
  if err != nil { panic(err) }
  _, err = fmt.Fprintf(sLog, "closed %s, %s %.1fm\n", aClient, aData, time.Since(aStart).Minutes())
  if err != nil { panic(err) }
  err = WriteSync(sDirname+"/timer/"+aClient, []byte("closed"), os.O_APPEND|os.O_WRONLY, 0)
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


