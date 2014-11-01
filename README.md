pacman-watch
============

Monitor Arch Linux updates for trouble.

Original discussion at https://github.com/networkimprov/arch-packages/issues/22

###Server

$ go build service.go  
$ ./service test  

Browse to http://localhost:4321/

Issues:  
There is a race condition where reqClose and timeUp modify the /timer/client file.  
Concurrent writes to sClient map[string]*tClient are not safe.  

Config file:

    "Http": host:port for listener
    "Password": http requests must contain this
    "Wait": integer time in seconds to wait for close message before sending email
    "To": should point to a mailing list
    "From": email address of sender
    "Message": should probably be removed from config

###Client

