$ErrorActionPreference = "Stop"

go build -ldflags "-H windowsgui" -o soundboard.exe .
if ($LASTEXITCODE -ne 0) {
    exit $LASTEXITCODE
}
