# EMA (Electromagnetic Assistant)

EMA (ElectroMagnetic Assistant) is a playground for testing out various AI
agents (LLMs) and interfacing with them. It's my way to connect different
speech-to-text, tex-to-speech and LLMs, see how they interact and perform.

The aim is to see how far I can push the Natural Language Interface.

## Why EMA?

Mostly becuase JARVIS is [way
overused](https://github.com/search?q=jarvis&type=repositories).

## How to run EMA?

You are going to need portaudio installed.

```bash
# Arch Linux
pacman -S portaudio

# Ubuntu
apt install portaudio19-dev

# MacOS
brew install portaudio
```

You will need to create the `.env` file, check [.env.example](.env.example)
for all the required variables.

Check [Makefile](Makefile) for all the available commands, but if you just want
to rn the app, use:

```bash
make run
```

To see logs, run:

```bash
tail -f tmp/log
```

## STT integrations

- Soniox real-time STT client docs: [core/speechtotext/soniox/README.md](core/speechtotext/soniox/README.md)

## Why Go?

Primarily it is what I use at work. The benefits are, it is low level enough to
do any audio processing (if needed) without having to drop into WASM or low-level
JS, or deal with the browser for UI. It is also high level enough so that I
don't have to deal with memory management. The added is out of the box
concurrency.
