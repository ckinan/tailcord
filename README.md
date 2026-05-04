# ckdiscord
A Discord client written in Go

The goal of this project is to create a really simple Discord client in the terminal that shows messages on a given channel.

## Motivation
Motivation: I use Discord as my notification server pushed by some microservices I have in my side projects. While having Discord app in my phone is useful when I am not at desk, I, more often, check the notifications (or some times alerts) in my desktop, and i like having it in a specific zone of my monitor always on top.

BUT, since I only use Discord for notification/alerts, feels like a good side project to create a minimalistic Discord client to optimize my machine's resources usage. I literally only need to show messages (text messages) from a given channel. So ideally, this tool should be as simple as just run it like:

```
ckdiscord --channel=lab-alerts
```

