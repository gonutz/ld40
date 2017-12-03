go get github.com/gonutz/blob/cmd/blob
mkdir temp_blob
copy *.png temp_blob
blob -folder=temp_blob -out=data.blob
del /Q temp_blob\*
rmdir temp_blob