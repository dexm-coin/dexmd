sudo rm -rf .dexm*

timestamp=$(date +%s)
timestamp=$((timestamp+110))
echo $timestamp
python timestamp.py $timestamp 1
scp main.go antoniogroza@35.211.241.218:/home/antoniogroza/go/src/github.com/dexm-coin/dexmd/
scp main.go root@142.93.117.17:/root/go/src/github.com/dexm-coin/dexmd
scp main.go root@68.183.22.198:/root/go/src/github.com/dexm-coin/dexmd

konsole -e ssh antoniogroza@35.211.241.218 "cd /home/antoniogroza/go/src/github.com/dexm-coin/dexmd/; ./server.sh satoshi3" &
konsole -e ssh root@142.93.117.17 "cd /root/go/src/github.com/dexm-coin/dexmd; ./server.sh w2" &
konsole -e ssh root@68.183.22.198 "cd /root/go/src/github.com/dexm-coin/dexmd; ./server.sh satoshi2" &

python timestamp.py $timestamp 2
echo "SLEEP"
sleep 80

konsole -e go run main.go sn w3