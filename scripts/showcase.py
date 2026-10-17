#!/usr/bin/env python3
"""Render VHS # Caption: annotations into a separate band below the terminal.

VHS has no caption command. Screenshot markers expose their actual frame numbers
in VHS's FFmpeg calls, which this script records through a temporary wrapper.
Unlike adding up Sleep commands, this stays synchronized across Wait and Hide.
Tested with VHS 0.12.1; an unknown screenshot layout fails rather than guessing.
"""

import argparse
import json
import os
from pathlib import Path
import re
import shlex
import shutil
import subprocess
import sys


def prepare_tape(source, work):
    lines = [f'Output "{work / "raw.mp4"}"']
    captions = []
    hidden = False
    for line in source.splitlines():
        stripped = line.strip()
        if stripped.startswith('Output '):
            continue
        if stripped.startswith('Set LoopOffset ') and stripped.split()[2] not in ('0', '0%'):
            raise ValueError('Caption markers do not support LoopOffset')
        if stripped.startswith('Set PlaybackSpeed ') and float(stripped.split()[2]) != 1:
            raise ValueError('Caption markers require PlaybackSpeed 1')
        if stripped == 'Hide':
            hidden = True
        elif stripped == 'Show':
            hidden = False
        if stripped.startswith('# Caption:'):
            if hidden:
                raise ValueError('A caption cannot start while recording is hidden')
            text = stripped.removeprefix('# Caption:').strip()
            if not text:
                raise ValueError('Caption text cannot be empty')
            lines.append(f'Screenshot "{work / f"caption-{len(captions):04d}.png"}"')
            captions.append(text)
        else:
            lines.append(line)
    if not captions:
        raise ValueError('No # Caption: annotations found in the tape')
    return '\n'.join(lines) + '\n', captions


def caption_frame(arguments):
    if not arguments:
        return None
    cue = re.fullmatch(r'caption-(\d{4})\.png', Path(arguments[-1]).name)
    if cue is None:
        return None
    for argument in arguments:
        frame = re.fullmatch(r'frame-text-(\d+)\.png', Path(argument).name)
        if frame:
            return int(cue[1]), int(frame[1])
    raise ValueError('VHS screenshot frame layout changed; cannot synchronize captions')


def recording_framerate(source):
    # VHS's margin overlay can export at 25fps even when capturing at 24fps.
    # Screenshot frame numbers belong to the capture clock, not that output.
    match = re.search(r'^Set Framerate (\d+)\s*$', source, re.MULTILINE)
    return int(match[1]) if match else 50


def make_subtitles(captions, frames, framerate, duration):
    if set(frames) != set(range(len(captions))) or framerate <= 0:
        raise ValueError('Missing caption frame markers')
    starts = [(frames[i] - 1) / framerate for i in range(len(captions))]
    boundaries = starts + [duration]
    if starts[0] < 0 or any(a >= b for a, b in zip(boundaries, boundaries[1:])):
        raise ValueError('Caption frame markers are out of order or outside the video')

    def timestamp(seconds):
        milliseconds = round(seconds * 1000)
        hours, milliseconds = divmod(milliseconds, 3_600_000)
        minutes, milliseconds = divmod(milliseconds, 60_000)
        seconds, milliseconds = divmod(milliseconds, 1000)
        return f'{hours:02}:{minutes:02}:{seconds:02},{milliseconds:03}'

    return ''.join(
        f'{i + 1}\n{timestamp(start)} --> {timestamp(end)}\n{text}\n\n'
        for i, (start, end, text) in enumerate(zip(starts, boundaries[1:], captions))
    )


def forward_ffmpeg(arguments):
    marker = caption_frame(arguments)
    if marker is not None:
        with open(os.environ['RDTUI_SHOWCASE_TIMINGS'], 'a', encoding='utf-8') as log:
            log.write(json.dumps(marker) + '\n')
    executable = os.environ['RDTUI_SHOWCASE_FFMPEG']
    os.execv(executable, [executable, *arguments])


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('tape', nargs='?', default='docs/assets/rdtui-showcase.tape')
    parser.add_argument('--vhs', default='vhs')
    parser.add_argument('--work-dir', default='bin/showcase')
    parser.add_argument('--captions-only', action='store_true',
                        help='Reuse a recording with the exact same prepared tape')
    args = parser.parse_args()
    ffmpeg, ffprobe = shutil.which('ffmpeg'), shutil.which('ffprobe')
    if not ffmpeg or not ffprobe:
        parser.error('ffmpeg (with libass) and ffprobe must be installed')

    source = Path(args.tape).read_text(encoding='utf-8')
    outputs = [shlex.split(line)[1] for line in source.splitlines() if line.startswith('Output ')]
    if not outputs or Path(outputs[0]).suffix != '.mp4':
        parser.error('The first Output must be an MP4 path')
    output = Path(outputs[0]).resolve()
    work = Path(args.work_dir).resolve()
    work.mkdir(parents=True, exist_ok=True)
    tape, captions = prepare_tape(source, work)
    prepared = work / 'showcase.tape'
    timings = work / 'timings.jsonl'
    if args.captions_only:
        if prepared.read_text(encoding='utf-8') != tape:
            raise ValueError('The tape changed; make a new recording before reusing captions')
    else:
        prepared.write_text(tape, encoding='utf-8')
        timings.write_text('', encoding='utf-8')

    # Only the child VHS process sees the wrapper; never replace system FFmpeg.
    wrapper_dir = work / 'tools'
    wrapper_dir.mkdir(exist_ok=True)
    wrapper = wrapper_dir / 'ffmpeg'
    wrapper.write_text(
        '#!/bin/sh\nexec ' + shlex.join([sys.executable, str(Path(__file__).resolve()), '--ffmpeg'])
        + ' "$@"\n', encoding='utf-8',
    )
    wrapper.chmod(0o755)
    env = dict(os.environ, PATH=str(wrapper_dir) + os.pathsep + os.environ['PATH'],
               RDTUI_SHOWCASE_TIMINGS=str(timings), RDTUI_SHOWCASE_FFMPEG=ffmpeg)
    if not args.captions_only:
        subprocess.run([args.vhs, 'validate', str(prepared)], check=True, env=env)
        subprocess.run([args.vhs, str(prepared)], check=True, env=env)

    metadata = json.loads(subprocess.check_output([
        ffprobe, '-v', 'error', '-select_streams', 'v:0', '-show_streams', '-show_format',
        '-of', 'json', str(work / 'raw.mp4'),
    ]))
    stream = metadata['streams'][0]
    records = [json.loads(line) for line in timings.read_text().splitlines()]
    frames = dict(records)
    if len(frames) != len(records):
        raise ValueError('Duplicate caption markers')
    subtitles = make_subtitles(captions, frames, recording_framerate(source),
                               float(metadata['format']['duration']))
    (work / 'captions.srt').write_text(subtitles, encoding='utf-8')

    style = (f'PlayResX={stream["width"]},PlayResY={stream["height"] + 120},'
             'Fontname=Ubuntu Sans Mono,Fontsize=40,PrimaryColour=&H00F4D6CD,'
             'Outline=0,Shadow=0,Alignment=2,MarginV=40')
    filters = f"pad=iw:ih+120:0:0:color=0x11111b,subtitles=captions.srt:force_style='{style}'"

    def render(*arguments):
        subprocess.run([ffmpeg, '-hide_banner', '-loglevel', 'error', '-y', *arguments],
                       check=True, cwd=work)

    render('-i', 'raw.mp4', '-vf', filters, '-c:v', 'libx264', '-crf', '18',
           '-pix_fmt', 'yuv420p', '-movflags', '+faststart', '-threads', '2', '-an', 'captioned.mp4')
    output.parent.mkdir(parents=True, exist_ok=True)
    shutil.copyfile(work / 'captioned.mp4', output)
    output.with_suffix('.srt').write_text(subtitles, encoding='utf-8')
    print(f'Rendered {len(captions)} captions: {output}')
    print(f'Video and scene screenshots: {work}')


if __name__ == '__main__':
    try:
        if sys.argv[1:2] == ['--ffmpeg']:
            forward_ffmpeg(sys.argv[2:])
        else:
            main()
    except (ValueError, OSError, subprocess.CalledProcessError) as error:
        sys.exit(str(error))
