import sys

if sys.argv[2] == "1":
    with open("main.go") as f:
        f = f.readlines()
        f = ''.join(f)
        f = f.split("-- start\n")
        # print(f[1])
        timestamp = sys.argv[1]
        f[1] = f'''-- start
        PUBLIC_PEERSERVER = true
        TS                = uint64({timestamp})
        // -- start
        '''
        with open("main.go", 'w') as f2:
            f2.write(''.join(f))

if sys.argv[2] == "2":
    with open("main.go") as f:
        f = f.readlines()
        f = ''.join(f)
        f = f.split("-- start\n")
        timestamp = sys.argv[1]
        f[1] = f'''-- start
        PUBLIC_PEERSERVER = false
        TS                = uint64({timestamp})
        // -- start
        '''
        with open("main.go", 'w') as f2:
            f2.write(''.join(f))
