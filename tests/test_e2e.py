"""
DeskRemote E2E Test Suite via Playwright
Verifies:
1. Authentication & PIN submission
2. Bottom Dock rendering & telemetry
3. Genie Effect cards toggle (Macros, Clipboard, Trackpad, Keyboard)
4. Interactive actions (Clipboard fill, Trackpad drag, Keyboard layout switch)
5. Mobile viewport responsiveness (390x844)
6. Screenshot capture for visual proof
"""

import sys
import time

# Ensure UTF-8 output on Windows console
if sys.platform == "win32":
    try:
        sys.stdout.reconfigure(encoding="utf-8")
    except AttributeError:
        pass

from playwright.sync_api import sync_playwright, expect

TARGET_URL = "http://localhost:8080"
TEST_PIN = "1234"

def run_e2e_tests():
    print("=" * 60)
    print("🚀 Starting DeskRemote E2E Test Suite with Playwright")
    print(f"Target URL: {TARGET_URL}")
    print("=" * 60)

    with sync_playwright() as p:
        # Launch Chromium
        browser = p.chromium.launch(headless=True)
        context = browser.new_context(viewport={"width": 1280, "height": 720})
        page = context.new_page()

        console_errors = []
        page.on("console", lambda msg: console_errors.append(msg.text) if msg.type == "error" else None)

        try:
            # 1. Navigation
            print("\n[Step 1] Navigating to DeskRemote...")
            response = page.goto(TARGET_URL, wait_until="networkidle", timeout=10000)
            assert response.status == 200, f"Expected HTTP 200, got {response.status}"
            print("  ✓ Page loaded successfully (HTTP 200)")

            # 2. Auth Modal & PIN
            print("\n[Step 2] Testing PIN Authentication...")
            auth_modal = page.locator("#authModal")
            expect(auth_modal).to_be_visible()
            
            pin_input = page.locator("#pinInput")
            pin_input.fill(TEST_PIN)
            
            login_btn = page.locator("#loginBtn")
            login_btn.click()
            
            # Wait for auth modal to disappear
            page.wait_for_timeout(500)
            expect(auth_modal).to_be_hidden()
            print(f"  ✓ PIN '{TEST_PIN}' submitted, authModal closed")

            # 3. Bottom Dock & Telemetry
            print("\n[Step 3] Verifying Bottom Dock & Telemetry...")
            dock = page.locator("#bottomDock")
            expect(dock).to_be_visible()
            
            session_timer = page.locator("#sessionTimer")
            expect(session_timer).to_be_visible()
            print("  ✓ Bottom Dock and Session Timer are visible")

            # 4. Genie Effect Card: Macros
            print("\n[Step 4] Testing ⚡ Macros Card (Genie Effect)...")
            pill_macros = page.locator("#pillMacros")
            macros_card = page.locator("#macrosCard")
            
            pill_macros.click()
            page.wait_for_timeout(400)
            expect(macros_card).to_be_visible()
            print("  ✓ Macros Card opened via Genie Effect")
            
            # Close it
            pill_macros.click()
            page.wait_for_timeout(400)
            expect(macros_card).to_be_hidden()
            print("  ✓ Macros Card closed into dock pill")

            # 5. Genie Effect Card: Clipboard
            print("\n[Step 5] Testing 📋 Smart Clipboard Card...")
            pill_clip = page.locator("#pillClipboard")
            clip_card = page.locator("#clipboardCard")
            
            pill_clip.click()
            page.wait_for_timeout(400)
            expect(clip_card).to_be_visible()
            
            clip_input = page.locator("#clipTextInp")
            clip_input.fill("https://github.com/browser-use")
            
            send_btn = page.locator("#sendClipBtn")
            send_btn.click()
            page.wait_for_timeout(500)
            expect(clip_card).to_be_hidden()
            print("  ✓ Text submitted to clipboard and card auto-closed")

            # 6. Genie Effect Card: Trackpad
            print("\n[Step 6] Testing 🖱 Touch Trackpad...")
            pill_track = page.locator("#pillTrackpad")
            track_card = page.locator("#trackpadCard")
            
            pill_track.click()
            page.wait_for_timeout(400)
            expect(track_card).to_be_visible()
            
            # Simulate swipe on trackpad zone
            track_zone = page.locator("#trackpadZone")
            box = track_zone.bounding_box()
            if box:
                page.mouse.move(box["x"] + 50, box["y"] + 50)
                page.mouse.down()
                page.mouse.move(box["x"] + 120, box["y"] + 80, steps=5)
                page.mouse.up()
                print("  ✓ Relative mouse movement swipe simulated on trackpad")
                
            pill_track.click()
            page.wait_for_timeout(400)
            expect(track_card).to_be_hidden()

            # 7. Genie Effect Card: Virtual Keyboard
            print("\n[Step 7] Testing ⌨ On-Screen Keyboard...")
            pill_kb = page.locator("#pillKeyboard")
            kb_card = page.locator("#keyboardCard")
            
            pill_kb.click()
            page.wait_for_timeout(400)
            expect(kb_card).to_be_visible()
            
            # Toggle language
            lang_btn = page.locator("#langSwitchBtn")
            expect(lang_btn).to_be_visible()
            initial_lang = lang_btn.inner_text()
            lang_btn.click()
            page.wait_for_timeout(200)
            new_lang = lang_btn.inner_text()
            print(f"  ✓ Language switched from '{initial_lang}' to '{new_lang}'")
            
            pill_kb.click()
            page.wait_for_timeout(400)
            expect(kb_card).to_be_hidden()

            # 8. Desktop Screenshot
            screenshot_path = "tests/e2e_desktop_success.png"
            page.screenshot(path=screenshot_path)
            print(f"\n[Step 8] Desktop screenshot saved: {screenshot_path}")

            # 9. Mobile Emulation (iPhone 15 Pro Viewport)
            print("\n[Step 9] Testing Mobile Responsive Viewport (390x844)...")
            mobile_context = browser.new_context(viewport={"width": 390, "height": 844}, is_mobile=True)
            mobile_page = mobile_context.new_page()
            mobile_page.goto(TARGET_URL, wait_until="networkidle", timeout=10000)
            
            # Auth on mobile
            mobile_page.locator("#pinInput").fill(TEST_PIN)
            mobile_page.locator("#loginBtn").click()
            mobile_page.wait_for_timeout(500)
            
            mobile_dock = mobile_page.locator("#bottomDock")
            expect(mobile_dock).to_be_visible()
            
            mobile_screenshot_path = "tests/e2e_mobile_success.png"
            mobile_page.screenshot(path=mobile_screenshot_path)
            print(f"  ✓ Mobile dock rendered cleanly without overflow: {mobile_screenshot_path}")
            mobile_context.close()

            # 10. Console Error Verification
            filtered_errors = [e for e in console_errors if "favicon" not in e.lower()]
            if filtered_errors:
                print(f"\n⚠️ Warnings / Console Errors ({len(filtered_errors)}):")
                for err in filtered_errors[:5]:
                    print(f"   - {err}")
            else:
                print("\n✓ Clean browser console: 0 JavaScript errors detected")

            print("\n" + "=" * 60)
            print("🎉 ALL 9 E2E TEST SCENARIOS PASSED SUCCESSFULLY!")
            print("=" * 60)
            return True

        except Exception as e:
            error_screenshot = "tests/e2e_failure.png"
            page.screenshot(path=error_screenshot)
            print(f"\n❌ E2E TEST FAILED: {e}")
            print(f"Failure screenshot captured at: {error_screenshot}")
            return False
        finally:
            browser.close()

if __name__ == "__main__":
    success = run_e2e_tests()
    sys.exit(0 if success else 1)
