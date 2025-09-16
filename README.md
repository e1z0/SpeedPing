# SpeedPing

**SpeedPing** is a lightweight, cross-platform network diagnostics tool built with Go and [MiQT](https://github.com/mappu/miqt).  
It combines three essential functions into one clean interface:

- **Ping Monitor** — keep track of multiple hosts simultaneously, with realtime charts showing latency and packet loss.  
- **Speed Test** — run bandwidth tests powered by bundled [iperf3](https://github.com/esnet/iperf) binaries, with live graphs of throughput.
- **Traceroute** - SpeedPing provides an interactive and animated view of network hops and latency in real time.

The app is designed to be small, fast, and ergonomic — perfect for homelabbers, sysadmins, and anyone who needs quick visibility into network performance.

---

## Screenshots

<img width="600" alt="SpeedPing-PingTest1" src="https://github.com/user-attachments/assets/abdab58d-8400-48cb-90f1-a90dbf6f9947" /> 
<img width="600" alt="SpeedPing-PingTest2" src="https://github.com/user-attachments/assets/1713e719-81c3-46d2-b536-1dd94c4c498c" />
<img width="600" alt="SpeedPing-Iperf" src="https://github.com/user-attachments/assets/8058cca2-76c8-4480-ba89-920870d67dd8" />
<img width="600" alt="SpeedPing-Traceroute" src="https://github.com/user-attachments/assets/04e451a3-7c3a-42e9-83b8-faa43650c653" />

---

## ✨ Features

- **Ping Tab**
  - Add multiple hosts and watch their latency in realtime.
  - Scrollable host list and per-host graph.
  - Packet loss and jitter tracking.

- **Speed Test Tab**
  - Automatic detection of bundled iperf3 binary.
  - Full set of test options: duration, interval, parallel streams, reverse (`-R`), bidirectional, UDP mode.
  - Realtime Mbps graph with hover tooltips.
  - Status indicators and Start/Stop controls.

- **About Tab**
  - Shows version/build info and build date.
  - System information snapshot (Go runtime, OS/arch, CPU count).
  - Quick links: GitHub, license page.
  - Shortcuts to open config and logs folder, copy system info.
- **Traceroute Tab**
  - Displays each hop in a traceroute as a node on a **latency vs. hop graph**.
  - Draws **connecting paths** between responsive hops with a neon-styled line.
  - Uses **color coding** and **animations** to make the traceroute intuitive and visually engaging.

---

## 📦 Installation

Prebuilt binaries for **Windows**, **macOS** (Intel & Apple Silicon), and **Linux** will be published in [Releases](../../releases).

Alternatively, build from source (macOS):

```bash
git clone https://github.com/e1z0/SpeedPing
cd SpeedPing
make
```

### iperf3 binary

SpeedPing expects to find an `iperf3` binaries inside the `./iperf/` directory (bundled in release archives) or in operating system PATH.  

---

# iperf3 static binaries

https://github.com/userdocs/iperf3-static/releases/tag/3.19.1
