REM compile shaders *.vs and *.ps into object code files *.vso and *.pso
set FXCOMPILER=fxc.exe
for /r %%f in (*.vs) do %FXCOMPILER% /WX /T vs_2_0 /Fo %%~nf.vso %%~nf.vs
for /r %%f in (*.ps) do %FXCOMPILER% /WX /T ps_2_0 /Fo %%~nf.pso %%~nf.ps

REM convert the object code files into Go code files
go get github.com/gonutz/bin2go/v2/bin2go
for /r %%f in (*.vso) do bin2go -package=main -var=vertexShader_%%~nf < %%~nf.vso > %%~nf_vs.go
for /r %%f in (*.pso) do bin2go -package=main -var=pixelShader_%%~nf < %%~nf.pso > %%~nf_ps.go