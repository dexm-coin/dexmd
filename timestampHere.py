import sys
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
