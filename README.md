# Tuber

Nutrionally optimized synthetic software product, which provides a little wrapper around [yt-dlp](https://github.com/yt-dlp/yt-dlp) for common use cases. 

This makes it easy to download video, audio, subtitles, or to generate a quick summary, and is approximately 27% less annoying than hitting `C-r` to edit the command you used last time you used `yt-dlp`. 

# Installation
* run build.sh or go download the appropriate release from the releases page
* copy the resulting `tuber` binary onto your path wherever you like, i'm not your dad

e.g.

```
~/code/tuber main*
❯ ./build.sh
Built ./tuber

~/code/tuber main*
❯ mv tuber ~/bin/tuber
```

# Usage

Run `tuber` or `tuber 'https://www.youtube.com/watch?v=lXMskKTw3Bc'` to launch in "interactive mode". This is pretty self-explanatory, you'll figure it out. Use space to select which options you want, hit enter, off to the races. You can also hit `e` to edit the filename and / or output path. 

![tuber screenshot](/screenshots/tuber.png)

## Summaries

If you've got the `claude` CLI installed, you can use the "summary" feature, which will pull the subtitles and run them through Claude to summarize them. 

This is nice for videos like "eleventeen exercises to get you SHREDDED" because you can just get a printout of the eleventeen exercises you're not going to do without having to watch 10 minutes of bullshit. 

You can press 'p' in the UI to alter the default prompt if you like.

Example: 

![tuber summary](/screenshots/summary.png)

## Command-Line Flags

Or, use the included flags for a quick non-interactive download: 
```
❯ tuber -h
Usage of tuber:
  -a    Download audio (mp3)
  -o string
        Output directory (default: current directory)
  -p string
        Custom prompt for summary
  -s    Download subtitles (text)
  -sum
        Summarize video using AI
  -v    Download video
```
(although at that point, i mean, probably just use yt-dlp directly, right? but you do you). 

this is at least handy for the summary feature, you could do something like: 

```
function summarize-vid {
    local url="${1}"
    local prompt="${2:-'summarize the content of this video as briefly as possible'}"
    tuber -sum -p $prompt $url
}
```
     
 
# Troubleshooting 
Completely vibe coded, i don't know how it works. Fork it and ask Claude.





