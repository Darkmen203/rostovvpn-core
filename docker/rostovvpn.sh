#!/bin/sh
# sysctl -w net.ipv4.ip_forward=1
# sysctl -w net.ipv6.ip_forward=1

# ip rule add fwmark 1 table 100 ; 
# ip route add local 0.0.0.0/0 dev lo table 100 

# # CREATE TABLE
# iptables -t mangle -N rostovvpn

# # RETURN LOCAL AND LANS
# iptables -t mangle -A OUTPUT -j RETURN
# iptables -t nat -A rostovvpn --dport 2334 -j RETURN

# iptables -t mangle -A rostovvpn -d 10.0.0.0/8 -j RETURN
# iptables -t mangle -A rostovvpn -d 127.0.0.0/8 -j RETURN
# iptables -t mangle -A rostovvpn -d 169.254.0.0/16 -j RETURN
# iptables -t mangle -A rostovvpn -d 172.16.0.0/12 -j RETURN
# iptables -t mangle -A rostovvpn -d 192.168.50.0/16 -j RETURN
# iptables -t mangle -A rostovvpn -d 192.168.9.0/16 -j RETURN
# iptables -t mangle -A rostovvpn -d 224.0.0.0/4 -j RETURN
# iptables -t mangle -A rostovvpn -d 240.0.0.0/4 -j RETURN

# iptables -t mangle -A rostovvpn -p udp -j TPROXY --on-port 2334 --tproxy-mark 1
# iptables -t mangle -A rostovvpn -p tcp -j TPROXY --on-port 2334 --tproxy-mark 1

# # HIJACK ICMP (untested)
# # iptables -t mangle -A rostovvpn -p icmp -j DNAT --to-destination 127.0.0.1

# # REDIRECT
# iptables -t mangle -A PREROUTING -j rostovvpn


WORKDIR="/rostovvpn"
BIN="$WORKDIR/RostovVPNCli"

if [ -f "$WORKDIR/rostovvpn.json" ]; then
    "$BIN" run --config "$CONFIG" -d "$WORKDIR/rostovvpn.json"
else
    "$BIN" run --config "$CONFIG"
fi

