call build_shaders.bat
set GOARCH=386
go build -ldflags "-s -w -H=windowsgui" -o ld40.exe
call build_data_blob.bat
go get github.com/gonutz/payload/cmd/payload
payload -data=data.blob -exe=ld40.exe