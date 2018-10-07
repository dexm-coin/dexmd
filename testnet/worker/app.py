import requests, os, json
from threading import Thread
from time import sleep
import random

shard = random.randint(1, 10)
os.system("./dexmd mw wal.json " + str(shard))

wallet = json.loads(open('wal.json').read())
address = wallet["Address"]

timestamp = requests.get("http://coordinator:5000/send_money", params={
    "wallet": address
}
).text
timestamp = int(timestamp)

def send_dexmpos():
    sleep(timestamp+20)
    req = requests.get("http://coordinator:5000/send_money", params={
        "wallet": address
    })
    sleep(10)
    if req == "Sent":
        os.system("./dexmd mkt wal.json DexmPoS 20 2")
    else:
        print("failed")


thread = Thread(target=send_dexmpos)
thread.start()


os.system("./dexmd sn wal.json " + str(timestamp))

thread.join()
