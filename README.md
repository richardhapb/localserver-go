
A personal automation server built in Go that integrates with Spotify and manages device control across my home network. Some stack elements:

- RESTful API development using Go and Gin
- OAuth2 authentication with Spotify Web API
- System-level device control (wake/sleep/battery monitoring)
- Multi-device state management and synchronization
- Secure token handling and environment configuration

I use this together with Apple Shortcuts to automate many tasks in my daily routine, work, and music center. I use two accounts on Spotify, one for home and another for when I'm out; this allows usage in both places and keeps it synced.

## Core Features

### Spotify Integration
- Audio playback control across multiple devices
- Playlist management with queue synchronization  
- Volume control with device-specific handling
- Scheduled playback for alarms and sleep timers

### Device Management
- Wake-on-LAN support for remote device power control
- Sleep mode activation with screen locking
- Battery level monitoring
- Cross-platform command execution (macOS/Linux)

## Note

This is a personal project customized for my home automation needs. While not intended for general use.

The code is shared to showcase an approach for a local server for customized home automation. This project is under development to meet my new needs.

