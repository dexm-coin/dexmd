timestamp=$(date +%s)
timestamp=$((timestamp+60))
echo $timestamp
python timestampServer.py $timestamp
scp main.go antoniogroza@35.211.241.218:/home/antoniogroza/go/src/github.com/dexm-coin/dexmd/
python timestampHere.py $timestamp
echo "SLEEP 1 minute"
sleep 45

sudo rm -rf .dexm*
go run main.go sn w3