import os
from PIL import Image, ImageEnhance

def generate_icons():
    root = os.path.abspath(".")
    src_png = os.path.join(root, "server", "docs", "images", "logo.png")
    im = Image.open(src_png).convert("RGBA")
    bbox = im.getbbox()
    print("Source bbox:", bbox)

    cx = (bbox[0] + bbox[2]) / 2.0
    cy = (bbox[1] + bbox[3]) / 2.0
    # Golden balance crop size: 1450 gives ~0.5px margin on 16x16 tab icon
    crop_size = 1450
    crop_box = (
        int(cx - crop_size / 2.0),
        int(cy - crop_size / 2.0),
        int(cx + crop_size / 2.0),
        int(cy + crop_size / 2.0)
    )
    cropped = im.crop(crop_box)
    print("Cropped size:", cropped.size, "New bbox:", cropped.getbbox())

    # 1. Generate PWA icon (512x512)
    pwa_512 = cropped.resize((512, 512), Image.Resampling.LANCZOS)
    pwa_path = os.path.join(root, "web", "public", "assets", "pwa-icon.webp")
    pwa_512.save(pwa_path, format="WEBP", lossless=True)
    print("Saved:", pwa_path)

    # 2. Generate multi-resolution favicon.ico
    icon_sizes = [16, 32, 48, 64, 128, 256]
    frames = []
    for s in icon_sizes:
        frame = cropped.resize((s, s), Image.Resampling.LANCZOS)
        if s <= 32:
            r, g, b, a = frame.split()
            rgb = Image.merge("RGB", (r, g, b))
            rgb = ImageEnhance.Brightness(rgb).enhance(1.25)
            r2, g2, b2 = rgb.split()
            frame = Image.merge("RGBA", (r2, g2, b2, a))
        frames.append(frame)

    web_ico = os.path.join(root, "web", "public", "favicon.ico")
    frames[0].save(web_ico, format="ICO", sizes=[(s, s) for s in icon_sizes], append_images=frames[1:])
    print("Saved:", web_ico)

    nova_ico = os.path.join(root, "theme-nova", "public", "favicon.ico")
    frames[0].save(nova_ico, format="ICO", sizes=[(s, s) for s in icon_sizes], append_images=frames[1:])
    print("Saved:", nova_ico)

    # 3. Generate optimal favicon.svg with viewBox="324 354 1400 1400"
    # Preserves exactly ~0.5px sub-pixel margin so top circle is 100% complete and max-sized
    svg_content = '''<svg xmlns="http://www.w3.org/2000/svg" viewBox="324 354 1400 1400" role="img" aria-label="Sonar">
  <style>
    :root {
      color: #5B5BD6;
    }
    @media (prefers-color-scheme: dark) {
      :root {
        color: #7B7BFF;
      }
    }
    .probe-main {
      fill: currentColor;
      stroke: currentColor;
    }
  </style>
  <g class="probe-main">
    <!-- Concentric circular edges with thicker stroke for small sizes -->
    <path d="M 1094 1555
             A 619 619 0 1 0 954 1555
             L 940 1476.5
             A 543 543 0 1 1 1108 1476.5 Z"
          stroke-width="16" stroke-linejoin="round"/>

    <!-- Pulse traces with bold stroke for crisp rendering on 16x16 tab icons -->
    <path d="M 520 927 H 619 L 653 819 L 701 1014 L 730 927 H 770
             M 1278 927 H 1318 L 1347 1014 L 1395 819 L 1429 927 H 1528"
          fill="none" stroke-width="48" stroke-linecap="round" stroke-linejoin="round"/>

    <!-- Center probe head and needle tip -->
    <path fill-rule="evenodd" stroke="none"
          d="M 1118 1111
             A 204 204 0 1 0 930 1111
             C 949 1120 960 1137 963 1165
             L 1018 1697
             Q 1024 1712 1030 1697
             L 1085 1165
             C 1088 1137 1099 1120 1118 1111 Z
             M 1024 817
             A 113 113 0 1 0 1024 1043
             A 113 113 0 1 0 1024 817 Z"/>
  </g>
</svg>
'''
    favicon_svg_path = os.path.join(root, "web", "public", "assets", "favicon.svg")
    with open(favicon_svg_path, "w", encoding="utf-8") as f:
        f.write(svg_content)
    print("Saved:", favicon_svg_path)

if __name__ == "__main__":
    generate_icons()
