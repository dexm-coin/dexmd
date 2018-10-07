# if [ ! -d "Dexm" ]; then
#     mkdir Dexm
# fi
# cd Dexm

command -v go >/dev/null 2>&1 || { echo >&2 "Install golang before start"; exit 1; }
# command -v go >/dev/null 2>&1 || { echo >&2 "Install git before start"; exit 1; }

# echo "Download of Dexm repository"
# git clone https://github.com/dexm-coin/dexmd.git
go get -u github.com/golang/protobuf/{proto,protoc-gen-go}

echo "Downloading the dependency of the project"
go get -d ./...

go build -o dexm