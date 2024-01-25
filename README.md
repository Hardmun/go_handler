# go_handler
$Env:GOOS = "linux"; $Env:GOARCH = "amd64"
go build -o httpHandler .


git fetch --all
git reset --hard origin/main

[Unit]
Description=http Handler
After=multi-user.target

[Service]
WorkingDirectory=/home/someuser
ExecStart= ""

[Install]
WantedBy=multi-user.target