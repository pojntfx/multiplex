# Multiplex

![Logo](./docs/logo-readme.png)

Watch torrents with your friends.

[![Flatpak CI](https://github.com/pojntfx/multiplex/actions/workflows/flatpak.yaml/badge.svg)](https://github.com/pojntfx/multiplex/actions/workflows/flatpak.yaml)
![Go Version](https://img.shields.io/badge/go%20version-%3E=1.18-61CFDD.svg)
[![Go Reference](https://pkg.go.dev/badge/github.com/pojntfx/multiplex.svg)](https://pkg.go.dev/github.com/pojntfx/multiplex)
[![Matrix](https://img.shields.io/matrix/multiplex:matrix.org)](https://matrix.to/#/#multiplex:matrix.org?via=matrix.org)

## Overview

Multiplex is an app to watch torrents together, providing an experience similar to Apple's [SharePlay](https://support.apple.com/en-us/HT212823) and Amazon's [Prime Video Watch Party](https://www.amazon.com/adlp/watchparty).

It enables you to ...

- **Stream any torrent**: By utilizing the mpv video player, Multiplex has support for a wide range of video and audio formats.
- **Synchronize playback between remote peers**: Thanks to [weron](https://github.com/pojntfx/weron), Multiplex can be used to host online watch parties by synchronizing playback position, magnet links and other data between peers.
- **Circumvent BitTorrent protocol censorship**: By splitting the core [hTorrent HTTP to BitTorrent gateway](https://github.com/pojntfx/htorrent) and UI into two separate projects, Multiplex can be used without having to connect a client to the BitTorrent protocol.

## Installation

On Linux, Multiplex is available on Flathub:

<a href="https://flathub.org/apps/details/com.pojtinger.felicitas.Multiplex"><img src="https://flathub.org/assets/badges/flathub-badge-en.png" alt="Flathub badge" width="200"/></a>

For other platforms, see [contributing](#contributing); development builds are also available on [GitHub releases](https://github.com/pojntfx/multiplex/releases/tag/release-main).

## Usage

### 1. Start Streaming a Torrent

To get started, first find a [magnet link](https://en.wikipedia.org/wiki/Magnet_URI_scheme) that you want to stream. There are many sites on the internet to find them; check out [webtorrent.io/free-torrents](https://webtorrent.io/free-torrents) for some copyright-free torrents to try out. Once you've found one, launch Multiplex and enter the link:

<p align="center">
  <img alt="Starting the app" src="./docs/launch-app.png" />
</p>

![Initial start screen with link entered](./docs/link-entered.png)

Note that Multiplex will prompt you to install the [mpv media player](https://mpv.io/) if you don't already have it installed; to continue, please do so:

<p align="center">
  <img alt="Prompt to install mpv" src="./docs/install-mpv.png" />
</p>

Next, select the file you want to stream; note that only media files are supported:

![Media selection](./docs/media-selection.png)

Finally, confirm that you have the right to stream the media you've selected. Note that many countries have copyright restrictions - in that case, please [take appropriate measures to protect yourself](https://sec.eff.org/topics/VPN):

![Confirmation screen](./docs/confirmation.png)

Note that you can also choose to stream without downloading! Provided that the underlying media file supports streaming playback (such as `.mkv` files), this allows you to start playing the media immediately, without having to wait for it to download completely:

<p align="center">
  <img alt="Option to stream without downloading" src="./docs/stream-without-downloading.png" />
</p>

After you've given your consent, playback will start, and you can enjoy the media you've selected:

![Playback screen](./docs/playback.png)

## 2. Ask Friends to Join

While consuming media on your own can be fun, doing so with friends or your SO is always better. I built Multiplex to enjoy media together with my partner, but due to COVID and the Atlantic ocean we're unable to do so in person all the time - this app intents to bridge that gap. To ask someone to join, click on the people button in the top right, and copy the [stream code](https://github.com/pojntfx/multiplex/wiki/Stream-Codes):

![Join screen](./docs/join.png)

This stream code can now be entered by the person that wants to watch the media with you. There is no technical limit on how many people can join the session, so feel free to invite as many as you want!

![Entering stream codes](./docs/enter-stream-code.png)

After the person that wants to join has entered the stream code, they need to confirm that they too have the right to stream the media; depending on your country, please ask them to [take appropriate measures to protect themselves](https://sec.eff.org/topics/VPN):

![Confirmation screen](./docs/confirmation.png)

Note that is recommended not to choose the option to stream without downloading when streaming with multiple people; while it is supported and buffering is synchronized across peers, it requires a very good internet connection for all peers in order for it to work smoothly. Once all peers have joined, you can start playback and enjoy the media together:

![Two peers synchronizing playback](./docs/sync-playback.png)

All play/pause events, seeking position etc. will be synchronized between all peers using [weron](https://github.com/pojntfx/weron), a peer-to-peer networking library.

## 3. Increase Privacy and Security

As noted above, the legality of consuming media from torrents depends on the country you're in. In most countries, following [these guidelines on VPNs from the Electronic Frontier Foundation](https://sec.eff.org/topics/VPN) will suffice, but Multiplex provides an additional option: **Remoting**.

Multiplex is built on [hTorrent](https://github.com/pojntfx/htorrent), an HTTP to BitTorrent gateway. Using remoting, it is possible to use a trusted server as a proxy to stream torrents from. This makes it possible to not only increase security for all peers without them having to take the appropriate measures themselves, but it can also increase the performance by caching the media on a single server with a good internet connection. To enable remoting, first [host a hTorrent gateway with basic authentication enabled](https://github.com/pojntfx/htorrent#1-start-a-gateway-with-htorrent-gateway) - be sure to set up TLS certificates to enable encryption, for example by using [Caddy](https://caddyserver.com/). Once you have a gateway set up, you can configure Multiplex to use in its preferences:

![Remoting preferences](./docs/prefs-remoting.png)

Be sure to ask the people who want to stream the media with you to also use the gateway.

For more preferences, see the [screenshots](#screenshots).

🚀 **That's it!** We hope you enjoy using Multiplex.

## Screenshots

Click on an image to see a larger version.

<a display="inline" href="./docs/initial.png?raw=true">
<img src="./docs/initial.png" width="45%" alt="Screenshot of the initial screen" title="Screenshot of the initial screen">
</a>

<a display="inline" href="./docs/media-selection.png?raw=true">
<img src="./docs/media-selection.png" width="45%" alt="Screenshot of the media selection screen" title="Screenshot of the media selection screen">
</a>

<a display="inline" href="./docs/confirmation.png?raw=true">
<img src="./docs/confirmation.png" width="45%" alt="Screenshot of the confirmation screen" title="Screenshot of the confirmation screen">
</a>

<a display="inline" href="./docs/playback.png?raw=true">
<img src="./docs/playback.png" width="45%" alt="Screenshot of the playback screen" title="Screenshot of the playback screen">
</a>

<a display="inline" href="./docs/audiotracks.png?raw=true">
<img src="./docs/audiotracks.png" width="45%" alt="Screenshot of the audio tracks screen" title="Screenshot of the audio tracks screen">
</a>

<a display="inline" href="./docs/subtitles.png?raw=true">
<img src="./docs/subtitles.png" width="45%" alt="Screenshot of the subtitles screen" title="Screenshot of the subtitles screen">
</a>

<a display="inline" href="./docs/join.png?raw=true">
<img src="./docs/join.png" width="45%" alt="Screenshot of the join screen" title="Screenshot of the join screen">
</a>

<a display="inline" href="./docs/sync-playback.png?raw=true">
<img src="./docs/sync-playback.png" width="45%" alt="Screenshot of two peers synchronizing playback" title="Screenshot of two peers synchronizing playback">
</a>

<a display="inline" href="./docs/prefs-playback.png?raw=true">
<img src="./docs/prefs-playback.png" width="45%" alt="Screenshot of the playback preferences" title="Screenshot of the playback preferences">
</a>

<a display="inline" href="./docs/prefs-remoting.png?raw=true">
<img src="./docs/prefs-remoting.png" width="45%" alt="Screenshot of the playback remoting preferences" title="Screenshot of the playback remoting preferences">
</a>

<a display="inline" href="./docs/prefs-sync.png?raw=true">
<img src="./docs/prefs-sync.png" width="45%" alt="Screenshot of the synchronization preferences" title="Screenshot of the synchronization preferences">
</a>

## Acknowledgements

- [Brage Fuglseth](https://bragefuglseth.dev/) contributed the icon.
- [mpv](https://mpv.io/) provides the media player.
- [diamondburned/gotk4](https://github.com/diamondburned/gotk4) provides the GTK4 bindings for Go.
- [diamondburned/gotk4-adwaita](https://github.com/diamondburned/gotk4-adwaita) provides the `libadwaita` bindings for Go.
- [hTorrent](https://github.com/pojntfx/htorrent) provides the torrent gateway.
- [weron](https://github.com/pojntfx/weron) provides the WebRTC library for playback synchronization.

## Contributing

To contribute, please use the [GitHub flow](https://guides.github.com/introduction/flow/) and follow our [Code of Conduct](./CODE_OF_CONDUCT.md).

To build and start a development version of Multiplex locally, run the following:

```shell
$ git clone https://github.com/pojntfx/multiplex.git
$ cd multiplex
$ go generate ./... # Also see https://github.com/dennwc/flatpak-go-mod for updating the Flatpak manifest with Go dependencies and https://gist.github.com/pojntfx/6733a6aaff22d3dd0d91eefde399da42 for updating the icons
$ go run .
```

You can also open the project in [GNOME Builder](https://flathub.org/apps/org.gnome.Builder) and run it by clicking the play button in the header bar.

## License

Multiplex (c) 2023 Felicitas Pojtinger and contributors

SPDX-License-Identifier: AGPL-3.0
