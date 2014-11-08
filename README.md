pacman-watch
============

Monitor Arch Linux updates for trouble.

Original discussion at https://github.com/networkimprov/arch-packages/issues/22

###Server

$ go build watch.go  
$ ./watch test # creates files in ./  

Browse to http://localhost:4321/

Issues:  
Multiple emails may be sent if server is restarted after timeout.  
Concurrent writes to sClient map[string]*tClient are not safe.  

Config file:

    "Http": host:port for listener
    "Password": http requests must contain this
    "Wait": integer time in seconds to wait for close message before sending email
    "To": should point to a mailing list
    "From": email address of sender

###Client

