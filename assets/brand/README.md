# Overgent brand assets

The mark is an open `O` assembled from horizontal radar lines: coordination,
shared visibility, and traffic control in one shape. The source `logo.png` at
the repository root is the original visual reference; these assets are the
production-ready vector system derived from it.

- `overgent-mark.svg` is the freestanding mark for documentation and external
  surfaces. It switches between the product's light and dark inks with the
  viewer's color scheme. For an inline web treatment, use the React `BrandMark`
  component so CSS `color` controls it directly.
- `overgent-app-icon.svg` is the transparent master macOS app artwork. Like the
  documentation mark, it has no enclosing tile or background.
- `overgent-app-icon.icns` contains the raster sizes Finder, Dock, and Spotlight
  need. Regenerate it after changing the master geometry with:

  ```bash
  go run ./scripts/generate-brand-icons.go /tmp/overgent.iconset assets/brand/overgent-app-icon.icns
  ```

Keep the line geometry intact. Recolor with `color`/`currentColor`; use black on
light surfaces and white on dark surfaces. Do not add an enclosing tile or use
a CSS filter on the original PNG.
