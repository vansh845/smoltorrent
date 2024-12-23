# 🌐 SmolTorrent
A lightweight, custom-built BitTorrent client written in Go. Designed to handle `.torrent` files, connect to peers, download files, and manage partial downloads efficiently.

## 📋 Features
- **Parse .torrent files** 🧩
- **Peer discovery and connection** 🌎
- **Piece-by-piece downloading** 📥
- **Parallel downloads** for improved performance ⚡
- **Efficient resource management** to minimize overhead 🛠️

## 🛠️ Installation
```bash
# Clone the repository
git clone https://github.com/vansh845/smoltorrent.git
cd smoltorrent

# Build the project
go build

# Run the client
./smoltorrent path/to/file.torrent
```

## 🚀 Quick Start
```bash
# Start downloading a torrent
./smoltorrent example.torrent

# Monitor the download process
./smoltorrent --status
```

## 🧩 Supported Functionalities
| Feature                           | Description                                        |
|-----------------------------------|----------------------------------------------------|
| **Torrent Parsing**                | Parse and validate `.torrent` files                |
| **Tracker Communication**          | Communicate with trackers to get peer lists        |
| **Peer Connections**               | Connect and exchange data with peers               |
| **Piece Downloading**              | Download pieces of files and verify integrity      |
| **Sequential & Parallel Download** | Download files efficiently across multiple peers   |

## 🛠️ ToDo
- Reconnection Support - Retry peers on partial downloads 
- Progress Tracking    - Display download status and speed 

## ⚙️ How It Works
- Parses the `.torrent` file to extract metadata and trackers.
- Contacts trackers to retrieve peer information.
- Connects to multiple peers and downloads file pieces in parallel.
- Verifies each piece using SHA-1 hashes to ensure data integrity.


## 📚 Contributing
Feel free to submit issues and pull requests. Any improvements or new features are welcome!


