go install github.com/gonutz/blob/cmd/blob@latest
mkdir temp_blob
copy *.png temp_blob
blob -path=temp_blob -out=data.blob
del /Q temp_blob\*
rmdir temp_blob