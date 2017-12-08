# ld40

Ludum Dare 40 Entry - The more you have, the worse it is

# Controls

```
WASD     to move
Mouse    to look around
Space    jump
Shift    to run
Control  to sneak
F11      to toggle fullscreen
```

# Build

In order to build this game you must have the following prerequisites installed:

- [The Go programming language](https://golang.org/dl/)
  
  Make sure you have your `GOPATH` environment variable set and then add `%GOPATH%\bin` to your `PATH` environment variable.

- [Git](https://git-scm.com/downloads)

- [The Windows SDK](https://developer.microsoft.com/en-us/windows/downloads/windows-8-sdk)
  
  Try to locate the folder where it puts `fxc.exe` which is the DirectX shader compiler that we need.
  On my 64 bit Windows it is located at `C:\Program Files (x86)\Windows Kits\8.0\bin\x86`. Add this folder to your `PATH` as well. 

To build and run, say:

```
go get -u github.com/gonutz/ld40
cd %GOPATH%\src\github.com\gonutz\ld40
build.bat
ld40.exe
```
