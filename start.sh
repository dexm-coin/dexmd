sudo rm -rf .dexm*

timestamp=$(date +%s)
timestamp=$((timestamp+100))
echo $timestamp
python timestamp.py $timestamp 1
scp main.go antoniogroza@35.211.241.218:/home/antoniogroza/go/src/github.com/dexm-coin/dexmd/
python timestamp.py $timestamp 2
echo "SLEEP"
sleep 80

go run main.go sn w3