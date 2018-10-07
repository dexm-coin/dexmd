if [ "$1" != "" ]; then
    sudo rm -rf .dexm*
    go build
    sudo ./dexmd sn $1
else
    echo "Wallet empty"
fi