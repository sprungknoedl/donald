.PHONY: all clean

all:
	GOOS=linux GOARCH=amd64 go build -o donald-linux-amd64
	GOOS=linux GOARCH=arm go build -o donald-linux-arm
	GOOS=linux GOARCH=arm64 go build -o donald-linux-arm64
	GOOS=darwin GOARCH=amd64 go build -o donald-mac-amd64
	GOOS=darwin GOARCH=arm64 go build -o donald-mac-arm64
	GOOS=windows GOARCH=amd64 go build -o donald-windows-amd64.exe
	GOOS=windows GOARCH=arm go build -o donald-windows-arm.exe
	GOOS=windows GOARCH=arm64 go build -o donald-windows-arm64.exe

clean:
	rm donald-linux-amd64
	rm donald-linux-arm
	rm donald-linux-arm64
	rm donald-mac-amd64
	rm donald-mac-arm64
	rm donald-windows-amd64.exe
	rm donald-windows-arm.exe
	rm donald-windows-arm64.exe