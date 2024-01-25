# go_handler
$Env:GOOS = "linux"; $Env:GOARCH = "amd64"
go build -o httpHandler .


git fetch --all
git reset --hard origin/main

[Unit]
Description=http Handler
After=multi-user.target

[Service]
WorkingDirectory=/home/install/go/
ExecStart=/home/install/go/httpHandler

[Install]
WantedBy=multi-user.target
Alias=httpHandler.service
