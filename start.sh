sudo rm -rf .dexm*
# generate TS
timestamp=$(date +%s)
timestamp=$((timestamp+90))
echo $timestamp
python timestamp.py $timestamp 1
# git pull if i did some changes
ssh antoniogroza@35.211.241.218 "cd /home/antoniogroza/go/src/github.com/dexm-coin/dexmd/; git stash; git pull;"
ssh root@142.93.117.17 "cd /root/go/src/github.com/dexm-coin/dexmd; git stash; git pull;"
ssh root@68.183.22.198 "cd /root/go/src/github.com/dexm-coin/dexmd; git stash; git pull;"
# update TS
scp main.go antoniogroza@35.211.241.218:/home/antoniogroza/go/src/github.com/dexm-coin/dexmd/
scp main.go root@142.93.117.17:/root/go/src/github.com/dexm-coin/dexmd
scp main.go root@68.183.22.198:/root/go/src/github.com/dexm-coin/dexmd
# run the server
konsole -e ssh antoniogroza@35.211.241.218 "cd /home/antoniogroza/go/src/github.com/dexm-coin/dexmd/; ./server.sh satoshi3" &
konsole -e ssh root@142.93.117.17 "cd /root/go/src/github.com/dexm-coin/dexmd; ./server.sh w2" &
konsole -e ssh root@68.183.22.198 "cd /root/go/src/github.com/dexm-coin/dexmd; ./server.sh satoshi2" &
# wait and run mine
python timestamp.py $timestamp 2
echo "SLEEP"
sleep 60
konsole -e go run main.go sn w3