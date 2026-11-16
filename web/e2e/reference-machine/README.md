# Reference machine

`doctor-<date>.json` is the output of `vukem-video doctor --full --json` on the reference machine of SDD-008 section 8 (the machine of the spikes), with the hardware written next to it. Keep it with the performance numbers: a later run compares its own `doctor --json` (`system`, `ffmpeg.binary`, `ffmpeg.encoder`) with this file to tell whether the machine or the FFmpeg build changed before it compares the numbers (SDD-012 section 19.1).

To record a new one, run the command on the machine, add the hardware, and save it under the new date; never edit an old file.
