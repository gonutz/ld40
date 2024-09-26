@echo off
setlocal
pushd %~dp0

REM compile shaders *.vs and *.ps into object code files *.vso and *.pso
go install github.com/gonutz/dxc/cmd/dxc@latest
for /r %%f in (*.vs) do dxc -WX -T vs_2_0 < %%~nf.vs >%%~nf.vso 
for /r %%f in (*.ps) do dxc -WX -T ps_2_0 < %%~nf.ps >%%~nf.pso 

REM convert the object code files into Go code files
go install github.com/gonutz/bin2go/v2/bin2go@latest
for /r %%f in (*.vso) do bin2go -package=main -var=vertexShader_%%~nf < %%~nf.vso > %%~nf_vs.go
for /r %%f in (*.pso) do bin2go -package=main -var=pixelShader_%%~nf < %%~nf.pso > %%~nf_ps.go

popd
