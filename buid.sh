GOOS=windows GOARCH=amd64 go build -o TarkovIpRetriever.exe main.go
zip -9 TarkovIpRetriever.zip TarkovIpRetriever.exe
rm TarkovIpRetriever.exe
go build
strip -s TarkovIpRetriever