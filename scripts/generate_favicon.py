import os
from PIL import Image, ImageDraw, ImageFont

def draw_logo(draw, x0, y0, size):
    # Scale helper: 24px grid to size grid
    def s(val):
        return val * size / 24

    # Draw the squircle background (dark base color #12110F)
    bg_margin = 0.5
    bg_x0 = x0 + s(bg_margin)
    bg_y0 = y0 + s(bg_margin)
    bg_x1 = x0 + s(24 - bg_margin)
    bg_y1 = y0 + s(24 - bg_margin)
    bg_rx = s(5)
    
    draw.rounded_rectangle(
        [bg_x0, bg_y0, bg_x1, bg_y1],
        radius=bg_rx,
        fill=(18, 17, 15, 255) # #12110F
    )

    # Vertical pill on the left: x=4, y=6, w=3, h=12, rx=1.5
    left_x0 = x0 + s(4)
    left_y0 = y0 + s(6)
    left_x1 = x0 + s(4 + 3)
    left_y1 = y0 + s(6 + 12)
    left_rx = s(1.5)
    draw.rounded_rectangle(
        [left_x0, left_y0, left_x1, left_y1],
        radius=left_rx,
        fill=(236, 232, 223, 255) # #ECE8DF
    )

    # Top horizontal pill: x=10, y=6, w=10, h=2, rx=1
    top_x0 = x0 + s(10)
    top_y0 = y0 + s(6)
    top_x1 = x0 + s(10 + 10)
    top_y1 = y0 + s(6 + 2)
    top_rx = s(1)
    draw.rounded_rectangle(
        [top_x0, top_y0, top_x1, top_y1],
        radius=top_rx,
        fill=(236, 232, 223, 255) # #ECE8DF
    )

    # Middle horizontal pill: x=10, y=11, w=10, h=2, rx=1
    mid_x0 = x0 + s(10)
    mid_y0 = y0 + s(11)
    mid_x1 = x0 + s(10 + 10)
    mid_y1 = y0 + s(11 + 2)
    mid_rx = s(1)
    draw.rounded_rectangle(
        [mid_x0, mid_y0, mid_x1, mid_y1],
        radius=mid_rx,
        fill=(236, 232, 223, 255) # #ECE8DF
    )

    # Bottom horizontal pill: x=10, y=16, w=6, h=2, rx=1
    bot_x0 = x0 + s(10)
    bot_y0 = y0 + s(16)
    bot_x1 = x0 + s(10 + 6)
    bot_y1 = y0 + s(16 + 2)
    bot_rx = s(1)
    draw.rounded_rectangle(
        [bot_x0, bot_y0, bot_x1, bot_y1],
        radius=bot_rx,
        fill=(236, 232, 223, 255) # #ECE8DF
    )

    # Accent red dot: cx=18, cy=17, r=1.5
    dot_cx = x0 + s(18)
    dot_cy = y0 + s(17)
    dot_r = s(1.5)
    draw.ellipse(
        [dot_cx - dot_r, dot_cy - dot_r, dot_cx + dot_r, dot_cy + dot_r],
        fill=(196, 59, 43, 255) # #C43B2B
    )

def draw_glow(img, cx, cy, radius, color):
    # color is (r, g, b, max_a)
    draw = ImageDraw.Draw(img)
    r, g, b, max_a = color
    steps = 50
    for i in range(steps):
        frac = i / steps
        cur_r = radius * (1 - frac)
        alpha = int(max_a * frac)
        draw.ellipse(
            [cx - cur_r, cy - cur_r, cx + cur_r, cy + cur_r],
            fill=(r, g, b, alpha)
        )

def main():
    # Define output paths inside internal/webhook
    out_dir = "/home/fauzul/code/hackathon-lab/internal/webhook"
    os.makedirs(out_dir, exist_ok=True)

    # 1. Create a high-res canvas (1024x1024) with transparent background
    size = 1024
    img = Image.new("RGBA", (size, size), (0, 0, 0, 0))
    draw = ImageDraw.Draw(img)

    # Draw the logo on the favicon canvas
    draw_logo(draw, 0, 0, size)

    # Save as high-res PNG
    png_path = os.path.join(out_dir, "favicon.png")
    img_resized_png = img.resize((256, 256), resample=Image.Resampling.LANCZOS)
    img_resized_png.save(png_path, format="PNG")
    print(f"Saved favicon PNG to: {png_path}")

    # Save as multi-resolution ICO
    ico_path = os.path.join(out_dir, "favicon.ico")
    img.save(ico_path, format="ICO", sizes=[(16, 16), (32, 32), (48, 48), (256, 256)])
    print(f"Saved favicon ICO to: {ico_path}")

    # Save the SVG version directly
    svg_content = """<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" width="32" height="32" fill="none">
    <rect width="24" height="24" rx="5" fill="#12110F"/>
    <rect x="4" y="6" width="3" height="12" rx="1.5" fill="#ECE8DF"/>
    <rect x="10" y="6" width="10" height="2" rx="1" fill="#ECE8DF"/>
    <rect x="10" y="11" width="10" height="2" rx="1" fill="#ECE8DF"/>
    <rect x="10" y="16" width="6" height="2" rx="1" fill="#ECE8DF"/>
    <circle cx="18" cy="17" r="1.5" fill="#C43B2B"/>
</svg>"""
    svg_path = os.path.join(out_dir, "favicon.svg")
    with open(svg_path, "w") as f:
        f.write(svg_content)
    print(f"Saved favicon SVG to: {svg_path}")

    # 2. Generate the premium 1200x630 og-image.png
    print("Generating premium 1200x630 og-image.png...")
    og_w, og_h = 1200, 630
    og_img = Image.new("RGBA", (og_w, og_h), (18, 17, 15, 255)) # Base color #12110F

    # Draw ambient glowing backgrounds
    draw_glow(og_img, 1100, 0, 450, (196, 59, 43, 20)) # Red accent glow top-right
    draw_glow(og_img, 150, 580, 400, (236, 232, 223, 8)) # Off-white glow bottom-left

    # Create overlay for semi-transparent layers
    overlay = Image.new("RGBA", og_img.size, (0, 0, 0, 0))
    overlay_draw = ImageDraw.Draw(overlay)

    # Draw premium main card
    overlay_draw.rounded_rectangle(
        [80, 80, 1120, 550],
        radius=24,
        fill=(30, 28, 25, 180), # Card color #1E1C19 with ~70% opacity
        outline=(236, 232, 223, 20), # Border color #ECE8DF with ~8% opacity
        width=2
    )

    # Draw logo icon inside the card
    draw_logo(overlay_draw, 140, 160, 110)

    # Load premium fonts (try multiple paths to ensure compatibility)
    font_title = None
    font_subtitle = None
    font_tagline = None
    
    font_paths_serif = [
        "/usr/share/fonts/truetype/liberation/LiberationSerif-Bold.ttf",
        "/usr/share/fonts/truetype/dejavu/DejaVuSerif-Bold.ttf",
        "/usr/share/fonts/truetype/freefont/FreeSerifBold.ttf",
        "C:\\Windows\\Fonts\\georgiab.ttf",
        "C:\\Windows\\Fonts\\timesbd.ttf"
    ]
    font_paths_sans = [
        "/usr/share/fonts/truetype/liberation/LiberationSans-Regular.ttf",
        "/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
        "/usr/share/fonts/truetype/freefont/FreeSans.ttf",
        "C:\\Windows\\Fonts\\arial.ttf"
    ]
    font_paths_sans_bold = [
        "/usr/share/fonts/truetype/liberation/LiberationSans-Bold.ttf",
        "/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf",
        "/usr/share/fonts/truetype/freefont/FreeSansBold.ttf",
        "C:\\Windows\\Fonts\\arialbd.ttf"
    ]

    for p in font_paths_serif:
        if os.path.exists(p):
            try:
                font_title = ImageFont.truetype(p, 64)
                break
            except Exception:
                pass
    
    for p in font_paths_sans_bold:
        if os.path.exists(p):
            try:
                font_subtitle = ImageFont.truetype(p, 26)
                break
            except Exception:
                pass

    for p in font_paths_sans:
        if os.path.exists(p):
            try:
                font_tagline = ImageFont.truetype(p, 20)
                break
            except Exception:
                pass

    if font_title is None: font_title = ImageFont.load_default()
    if font_subtitle is None: font_subtitle = ImageFont.load_default()
    if font_tagline is None: font_tagline = ImageFont.load_default()

    # Draw Brand Title: "NewScriber"
    title_text = "NewScriber"
    try:
        # Use textlength to place the red dot exactly at the end
        title_w = overlay_draw.textlength(title_text, font=font_title)
    except AttributeError:
        # Fallback if textlength is not supported in older PIL versions
        title_w = len(title_text) * 38
    
    title_x, title_y = 280, 175
    overlay_draw.text((title_x, title_y), title_text, fill=(236, 232, 223, 255), font=font_title)
    
    # Draw Brand Red Dot at end of "NewScriber"
    dot_x = title_x + title_w + 5
    dot_y = title_y + 48
    overlay_draw.ellipse([dot_x, dot_y, dot_x + 12, dot_y + 12], fill=(196, 59, 43, 255))

    # Draw Tagline: "Your morning briefing. No app, no scroll, no noise."
    tagline_text = "Your morning briefing. No app, no scroll, no noise."
    overlay_draw.text((140, 340), tagline_text, fill=(236, 232, 223, 240), font=font_subtitle)

    # Draw description paragraph
    desc_text = "Dual-host conversational AI podcasts generated dynamically from your daily news feeds."
    overlay_draw.text((140, 400), desc_text, fill=(142, 137, 128, 255), font=font_tagline)

    # Draw simulated audio wave bars on the right side of the card
    wave_x_start = 810
    wave_y_mid = 315
    bar_width = 8
    bar_gap = 14
    # Heights of bars representing the soundwave
    bar_heights = [45, 80, 140, 200, 260, 220, 140, 180, 230, 170, 110, 75, 45]
    
    for i, h in enumerate(bar_heights):
        bx0 = wave_x_start + i * (bar_width + bar_gap)
        by0 = wave_y_mid - h / 2
        bx1 = bx0 + bar_width
        by1 = wave_y_mid + h / 2
        
        # Color: Use accent red for the middle bars to make it pop, off-white for others
        if 3 <= i <= 7:
            bar_color = (196, 59, 43, 255) # Accent red
        else:
            bar_color = (236, 232, 223, 160) # Muted off-white
            
        overlay_draw.rounded_rectangle([bx0, by0, bx1, by1], radius=4, fill=bar_color)

    # Composite the overlay onto the main image
    final_img = Image.alpha_composite(og_img, overlay)
    
    # Save the premium og-image.png
    og_image_path = os.path.join(out_dir, "og-image.png")
    final_img.save(og_image_path, format="PNG")
    print(f"Saved social preview image to: {og_image_path}")

if __name__ == "__main__":
    main()

