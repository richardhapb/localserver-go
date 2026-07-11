
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
- Device targeting: every playback request is routed to a specific reachable device by resolving its live `device_id`, so playback never falls back to the wrong device

### Device Management
- Wake-on-LAN support for remote device power control
- Sleep mode activation with screen locking
- Battery level monitoring
- Cross-platform command execution (macOS/Linux)

## API Endpoints

Server listens on `:9000`.

### Spotify (`/spotify`)
| Method | Path | Description |
| --- | --- | --- |
| GET | `/login?env=<home\|main>` | Start Spotify OAuth for an account |
| GET | `/callback` | OAuth redirect handler |
| GET | `/devices` | List every **reachable** device grouped by environment (`home`/`main`), regardless of what is playing |
| GET | `/play?device_name=<name>` | Resume playback on the named device |
| GET | `/pause?device_name=<name>` | Pause playback on the named device |
| GET | `/playlist?uri=<uri>&device_name=<name>&volume=<0-100>` | Play a playlist by URI on the named device |
| GET | `/search-playlist?query=<text>&device_name=<name>&volume=<0-100>` | Search a playlist and play the first match |
| GET | `/volume?percentage=<0-100>` | Set volume on the active device |
| GET | `/schedule?action=<alarm\|sleep>&time_millis=<epoch_ms>` | Schedule alarm/sleep playback |
| GET | `/transfer?to=<device_name>&volume=<0-100>` | Transfer current playback to another device/account |

Playback endpoints return the real outcome: `200` only when Spotify accepts the request,
`424` when the named device is not currently reachable (open the Spotify app on it), and
`502` with Spotify's status/body on any upstream failure.

### Management (`/manage`)
| Method | Path | Description |
| --- | --- | --- |
| GET | `/lamp` | Toggle the ESP32 relay lamp |
| POST | `/grammar` | Grammar/spelling review of posted text |

### Other
| Method | Path | Description |
| --- | --- | --- |
| GET | `/corrections` | List stored corrections |

## Note

This is a personal project customized for my home automation needs. While not intended for general use.

The code is shared to showcase an approach for a local server for customized home automation. This project is under development to meet my new needs.

