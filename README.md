pacman-watch
============

Monitor Arch Linux updates for trouble.

Original discussion at https://github.com/networkimprov/arch-packages/issues/22

$ go build service.go
$ ./service test

Browse to http://localhost:4321/

Email dispatch is untested. Server will crash if port 25 is blocked by your ISP.

