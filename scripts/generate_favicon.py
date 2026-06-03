import os
from PIL import Image, ImageDraw

def main():
    # 1. Create a high-res canvas (1024x1024) with transparent background
    size = 1024
    img = Image.new("RGBA", (size, size), (0, 0, 0, 0))
    draw = ImageDraw.Draw(img)

    # Scale helper: 24px grid to 1024px grid
    def s(val):
        return val * size / 24

    # 2. Draw the squircle background (dark base color #12110F)
    # Let's make it fill the space with a small margin so it sits nicely in a square.
    # Coordinates from 1.5 to 22.5 (leaves 1.5px margin on 24px grid)
    bg_margin = 0.5
    bg_x0 = s(bg_margin)
    bg_y0 = s(bg_margin)
    bg_x1 = s(24 - bg_margin)
    bg_y1 = s(24 - bg_margin)
    # A smooth corner radius
    bg_rx = s(5)
    
    draw.rounded_rectangle(
        [bg_x0, bg_y0, bg_x1, bg_y1],
        radius=bg_rx,
        fill=(18, 17, 15, 255) # #12110F
    )

    # 3. Draw the brand icon elements (off-white #ECE8DF and accent red #C43B2B)
    # Vertical pill on the left: x=4, y=6, w=3, h=12, rx=1.5
    left_x0 = s(4)
    left_y0 = s(6)
    left_x1 = s(4 + 3)
    left_y1 = s(6 + 12)
    left_rx = s(1.5)
    draw.rounded_rectangle(
        [left_x0, left_y0, left_x1, left_y1],
        radius=left_rx,
        fill=(236, 232, 223, 255) # #ECE8DF
    )

    # Top horizontal pill: x=10, y=6, w=10, h=2, rx=1
    top_x0 = s(10)
    top_y0 = s(6)
    top_x1 = s(10 + 10)
    top_y1 = s(6 + 2)
    top_rx = s(1)
    draw.rounded_rectangle(
        [top_x0, top_y0, top_x1, top_y1],
        radius=top_rx,
        fill=(236, 232, 223, 255) # #ECE8DF
    )

    # Middle horizontal pill: x=10, y=11, w=10, h=2, rx=1
    mid_x0 = s(10)
    mid_y0 = s(11)
    mid_x1 = s(10 + 10)
    mid_y1 = s(11 + 2)
    mid_rx = s(1)
    draw.rounded_rectangle(
        [mid_x0, mid_y0, mid_x1, mid_y1],
        radius=mid_rx,
        fill=(236, 232, 223, 255) # #ECE8DF
    )

    # Bottom horizontal pill: x=10, y=16, w=6, h=2, rx=1
    bot_x0 = s(10)
    bot_y0 = s(16)
    bot_x1 = s(10 + 6)
    bot_y1 = s(16 + 2)
    bot_rx = s(1)
    draw.rounded_rectangle(
        [bot_x0, bot_y0, bot_x1, bot_y1],
        radius=bot_rx,
        fill=(236, 232, 223, 255) # #ECE8DF
    )

    # Accent red dot: cx=18, cy=17, r=1.5
    dot_cx = s(18)
    dot_cy = s(17)
    dot_r = s(1.5)
    draw.ellipse(
        [dot_cx - dot_r, dot_cy - dot_r, dot_cx + dot_r, dot_cy + dot_r],
        fill=(196, 59, 43, 255) # #C43B2B
    )

    # Define output paths inside internal/webhook
    out_dir = "/home/fauzul/code/hackathon-lab/internal/webhook"
    os.makedirs(out_dir, exist_ok=True)

    # Save as high-res PNG
    png_path = os.path.join(out_dir, "favicon.png")
    img_resized_png = img.resize((256, 256), resample=Image.Resampling.LANCZOS)
    img_resized_png.save(png_path, format="PNG")
    print(f"Saved favicon PNG to: {png_path}")

    # Save as multi-resolution ICO
    ico_path = os.path.join(out_dir, "favicon.ico")
    img.save(ico_path, format="ICO", sizes=[(16, 16), (32, 32), (48, 48), (256, 256)])
    print(f"Saved favicon ICO to: {ico_path}")

    # Let's also save the SVG version directly
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

if __name__ == "__main__":
    main()
