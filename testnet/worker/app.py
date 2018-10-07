import requests, os, json
from threading import Thread
from time import sleep
import random

shard = random.randint(1, 10)
os.system("./dexmd mw wal.json " + str(shard))

wallet = json.loads(open('wal.json').read())
address = wallet["Address"]

timestamp = requests.get("http://35.211.241.218:5000/submit_addr", params={
    "wallet": address
}).text
timestamp = int(timestamp)


def send_dexmpos():
    print("waiting")
    sleep(timestamp+20)
    req = requests.get("http://35.211.241.218:5000/send_money", params={
        "wallet": address
    })
    # maybe i should wait more than 10 sec, should be around max 200 sec
    sleep(10)
    if req == "Sent":
        os.system("./dexmd mkt wal.json DexmPoS 20 2")
    else:
        print("failed")


thread = Thread(target=send_dexmpos)
thread.start()


os.system("./dexmd sn wal.json " + str(timestamp))

thread.join()
