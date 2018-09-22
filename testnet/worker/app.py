import requests, os, json

os.system("./dexmd mw wal.json")

wallet = json.loads(open('wal.json').read())

start = requests.get("http://coordinator:5000/submit_addr", params={
        "wallet" : wallet["Address"]
    }
).text

os.system("./dexmd sn wal.json " + start)