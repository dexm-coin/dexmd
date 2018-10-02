sudo rm -rf .dexm*

timestamp=$(date +%s)
timestamp=$((timestamp+130))
echo $timestamp
python timestamp.py $timestamp 1
scp main.go antoniogroza@35.211.241.218:/home/antoniogroza/go/src/github.com/dexm-coin/dexmd/
scp main.go root@142.93.117.17:/root/dexmd
scp main.go root@68.183.22.198:/root/dexmd

konsole -e ssh antoniogroza@35.211.241.218 "cd /home/antoniogroza/go/src/github.com/dexm-coin/dexmd/; ./server.sh" &
konsole -e ssh root@142.93.117.17 "cd /root/dexmd/; ./server.sh" &
konsole -e ssh root@68.183.22.198 "cd /root/dexmd/; ./server.sh" &

python timestamp.py $timestamp 2
echo "SLEEP"
sleep 100

konsole -e go run main.go sn w3