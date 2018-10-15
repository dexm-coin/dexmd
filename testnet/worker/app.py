import requests, os, json
from threading import Thread
from time import sleep, time
import random, os

# create the wallet
shard = random.randint(1, 5)
print("Using shard ", str(shard))
os.system("./dexmd mw wal.json " + str(shard))

wallet = json.loads(open('wal.json').read())
address = wallet["Address"]

validator = requests.get("http://35.211.241.218:5000/start_validator").text
validator = int(validator)

timestamp = 0
if validator != 0:
    timestamp = requests.get("http://35.211.241.218:5000/submit_addr", params={
        "wallet": "wallet"+str(validator)
    }).text
else:
    timestamp = requests.get("http://35.211.241.218:5000/submit_addr", params={
        "wallet": wallet
    }).text
timestamp = int(timestamp)

# wait for hackney to start
sleep(40)

def send_dexmpos():
    sleep(timestamp-time()+30)
    req = requests.get("http://35.211.241.218:5000/send_money", params={
        "wallet": address
    })
    # wait for merkle proof to actually have the balance to send money
    sleep(250)
    if req == "Sent":
        print("SENDING TO DEXMPOS")
        os.system("./dexmd mkt wal.json DexmPoS 20 2")
    else:
        print("REQUEST NOT SENT")


thread = Thread(target=send_dexmpos)
thread.start()

# check if you are a validator
if validator != 0:
    os.system("./dexmd sn wallet" + str(validator) + " " + str(timestamp))
else:
    # wait for all the other validator to start
    sleep(60)
    os.system("./dexmd sn wal.json " + str(timestamp))

thread.join()
