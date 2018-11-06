git stash
git pull
cd testnet/worker

docker build . -t dexmcoin/worker --no-cache && docker push dexmcoin/worker
docker service rm testnet

echo "START HACKNEY | sudo python app.py"
sleep 60

docker service create --name testnet dexmcoin/worker:latest
docker service scale testnet=100
while true
do
    clear
    docker service logs testnet
    sleep 5
done