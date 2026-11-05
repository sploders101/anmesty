# Amnesty

Break things. You're forgiven.

Amnesty is for system tinkerers. Imagine keeping Arch, but bringing in a taste
of Nix. Amnesty does this by creating an automated installer for Arch Linux and
leveraging the features of btrfs to create system snapshots, allowing safe
restore points when things break. Amnesty is unique in that it merges these two
ideas together, making re-imaging trivial and unobtrusive.


## Motivating Scenario

You build your Arch system. You are proud of this system. It works exactly the
way you want it to, and does nothing more. It looks fantastic, it's simple, it
boots fast, you could go on.

Fast forward 6 months...

You hate your system. It feels bloated. You've swapped PC parts several times
and forgot what drivers to remove. You've tried 30 different pieces of software
to solve a specific problem and completely forgot about 7 of them. You've tried
KDE, Hyprland, Sway, MangoWC, and more because things got buggy as updates came
and you wanted to find something a little more stable. Your system is now a
hodgepodge of failed experiments, and you know longer recognize every process
running on it.

You want to start over, but you forgot how you built your perfect system, and
you're not ready to give up your whole weekend to rebuild... so you stick with
what you have. The problem only gets worse...

Eventually, you rebuild, and everything feels fresh again, but it took all
weekend, and half your stuff still doesn't work. This time you document
everything, but don't understand what fixed that thing you spent all day
debugging. The cycle repeats...

You try Nix. It feels like a breath of fresh air. It was hard to figure out,
but everything is defined through a config, and all you need to do to rebuild
your system is hit save... and then you have to compile some code. You spend an
entire day trying to figure out how to run this stupid build command. The
project says all you need to do is `cargo build package`, but it fails. "No
network? What are you talking about? It says connected!" You begin researching
and find that Nix builds block internet access. You research offline build
solutions. They're all half-baked. You build on another machine and copy the
binary. 23 libs not found. You add the libs to your config. 23 libs not found.
You research how Nix actually works. You discover patchelf and it leaves a sour
taste in your mouth. "This is disgusting!" you shout. "Why would anyone ever do
this?!" you exclaim, with a disgusted look on your face. "Configuration-based
systems are a fantastic idea, but this seems like way too high a cost."

This was how I discovered, grew to love, and eventually came to hate Nixos.


## So what does Amnesty do?

Amnesty attempts to bring some of the beauty of Nix to Arch. It's a lot more
relaxed, and not quite so idealistic. The basic premise is as follows:

1. Make an extensible installer that can build your whole Arch system given a
root filesystem.
2. Pair this automated installer with containers so builds can happen while the
system is running.
3. Add a snapshotting feature so you can swap out your root filesystems
on-demand and keep plenty of working backups.
4. Freedom!

The basic idea is that if you can re-image your system without interrupting
your work, then you can periodically check that your configuration is accurate
and test it with relatively low risk. With snapshots, you can also do things
like say "Hold my beer. I wanna try something..." and restore your system when
everything crashes and burns.
