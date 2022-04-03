call build_shaders.bat

go install github.com/gonutz/rsrc@latest
rsrc -arch 386 -ico icon.ico -o rsrc_386.syso
rsrc -arch amd64 -ico icon.ico -o rsrc_amd64.syso

set GOARCH=386
go build -ldflags "-s -w -H=windowsgui" -o ld40.exe
set GOARCH=

call build_data_blob.bat
go install github.com/gonutz/payload/cmd/payload@latest
payload -data=data.blob -exe=ld40.exe