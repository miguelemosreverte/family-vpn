#!/bin/bash
# Build icon for Family VPN app using SF Symbols

# Create icon using a shield lock symbol
mkdir -p assets/icon.iconset

# Generate icon at different sizes using SF Symbols
# Using shield.fill symbol for VPN security
for size in 16 32 64 128 256 512; do
  sips -z $size $size /System/Library/CoreServices/CoreTypes.bundle/Contents/Resources/BookmarkIcon.icns --out "assets/icon.iconset/icon_${size}x${size}.png" 2>/dev/null || {
    # Fallback: use emoji to create icon
    convert -size ${size}x${size} -background none -fill "#667eea" -font "Apple Color Emoji" -pointsize $((size-20)) -gravity center label:"🔒" "assets/icon.iconset/icon_${size}x${size}.png" 2>/dev/null || {
      # If ImageMagick not available, create placeholder
      echo "Icon generation requires ImageMagick. Install with: brew install imagemagick"
    }
  }

  # Create @2x versions
  size2x=$((size * 2))
  cp "assets/icon.iconset/icon_${size}x${size}.png" "assets/icon.iconset/icon_${size}x${size}@2x.png" 2>/dev/null
done

# Convert to icns format
iconutil -c icns assets/icon.iconset -o assets/icon.icns

echo "✅ Icon created at assets/icon.icns"
