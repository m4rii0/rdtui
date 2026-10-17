import sys
import json
import shutil
import subprocess
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
import showcase


class ShowcaseTests(unittest.TestCase):
    @unittest.skipUnless(shutil.which('ffmpeg') and shutil.which('ffprobe'), 'FFmpeg required')
    def test_exports_captioned_h264_mp4_without_generating_a_gif(self):
        with tempfile.TemporaryDirectory() as directory:
            work = Path(directory)
            output = work / 'rdtui-showcase.mp4'
            source = f'Output "{output}"\nSet Framerate 24\n# Caption: Test caption\nSleep 1s\n'
            tape = work / 'input.tape'
            tape.write_text(source)
            prepared, _ = showcase.prepare_tape(source, work)
            (work / 'showcase.tape').write_text(prepared)
            (work / 'timings.jsonl').write_text('[0, 1]\n')
            subprocess.run(['ffmpeg', '-hide_banner', '-loglevel', 'error', '-y',
                            '-f', 'lavfi', '-i', 'color=c=black:s=640x360:r=24:d=1',
                            '-c:v', 'libx264', '-pix_fmt', 'yuv420p', str(work / 'raw.mp4')],
                           check=True)
            result = subprocess.run([sys.executable, str(Path(showcase.__file__)),
                                     '--captions-only', '--work-dir', str(work), str(tape)],
                                    capture_output=True, text=True)
            self.assertEqual(result.returncode, 0, result.stderr)
            metadata = json.loads(subprocess.check_output([
                'ffprobe', '-v', 'error', '-select_streams', 'v:0', '-show_entries',
                'stream=codec_name,pix_fmt,height', '-of', 'json', str(output)]))
            self.assertEqual(metadata['streams'][0],
                             {'codec_name': 'h264', 'pix_fmt': 'yuv420p', 'height': 480})
            data = output.read_bytes()
            self.assertLess(data.index(b'moov'), data.index(b'mdat'))
            self.assertIn('Test caption', output.with_suffix('.srt').read_text())
            self.assertEqual(list(work.glob('*.gif')), [])

    def test_marker_timing_uses_capture_rate_not_encoded_video_rate(self):
        self.assertEqual(showcase.recording_framerate('Set Framerate 24\n'), 24)
        self.assertEqual(showcase.recording_framerate('Set Width 1280\n'), 50)
        subtitles = showcase.make_subtitles(['First', 'Second'], {0: 1, 1: 1201},
                                            showcase.recording_framerate('Set Framerate 24\n'), 55)
        self.assertIn('00:00:50,000 --> 00:00:55,000', subtitles)

    def test_captions_become_screenshot_markers_without_changing_waits(self):
        tape, captions = showcase.prepare_tape(
            'Output old.mp4\nSet Framerate 24\nHide\nSleep 1s\nShow\n'
            '# Caption: [?] Discover the shortcuts.\nType "?"\n'
            'Wait+Screen /Shortcuts/\nSleep 2s\n',
            Path('/recording'),
        )
        self.assertEqual(captions, ['[?] Discover the shortcuts.'])
        self.assertIn('Output "/recording/raw.mp4"', tape)
        self.assertNotIn('Output old.mp4', tape)
        self.assertIn('Screenshot "/recording/caption-0000.png"', tape)
        self.assertIn('Wait+Screen /Shortcuts/', tape)

    def test_rejects_captions_while_recording_is_hidden(self):
        with self.assertRaisesRegex(ValueError, 'hidden'):
            showcase.prepare_tape('Hide\n# Caption: Not visible\n', Path('/recording'))

    def test_reads_actual_screenshot_frame_not_video_sequence_pattern(self):
        self.assertEqual(
            showcase.caption_frame(['-i', '/tmp/frame-text-00049.png', '-i',
                                    '/tmp/frame-cursor-00049.png',
                                    '/recording/caption-0001.png']),
            (1, 49),
        )
        self.assertIsNone(showcase.caption_frame(['-i', 'frame-text-%05d.png', 'raw.mp4']))

    def test_unknown_vhs_screenshot_layout_fails_instead_of_guessing(self):
        with self.assertRaisesRegex(ValueError, 'frame'):
            showcase.caption_frame(['-i', 'unknown.png', '/recording/caption-0000.png'])

    def test_subtitles_use_recorded_frames_and_cover_the_last_scene(self):
        subtitles = showcase.make_subtitles(['First', 'Second'], {0: 1, 1: 49}, 24, 5.25)
        self.assertEqual(subtitles,
                         '1\n00:00:00,000 --> 00:00:02,000\nFirst\n\n'
                         '2\n00:00:02,000 --> 00:00:05,250\nSecond\n\n')

    def test_missing_or_out_of_order_markers_fail(self):
        for frames in ({0: 1}, {0: 49, 1: 1}, {0: 1, 1: 1}):
            with self.subTest(frames=frames), self.assertRaises(ValueError):
                showcase.make_subtitles(['First', 'Second'], frames, 24, 5)

    def test_rejects_retiming_settings_that_would_invalidate_frame_numbers(self):
        for setting in ('Set LoopOffset 50%', 'Set PlaybackSpeed 2'):
            with self.subTest(setting=setting), self.assertRaises(ValueError):
                showcase.prepare_tape(setting + '\n# Caption: Example\nSleep 1s', Path('/recording'))


if __name__ == '__main__':
    unittest.main()
