package ui

// Height category constants for dynamic layout adjustment
const (
	HeightCategoryTooSmall   = -1 // <= 30 rows - show modal
	HeightCategorySmall      = 0  // 31-45 rows
	HeightCategoryMedium     = 1  // 46-50 rows
	HeightCategoryLarge      = 2  // 51-64 rows
	HeightCategoryExtraLarge = 3  // >= 65 rows (dynamic sizing)

	// MinTerminalHeight is the minimum terminal height required for ktop
	// Modal shows when height < MinTerminalHeight (i.e., <= 30)
	MinTerminalHeight = 31
)

// RectGetter is an interface for types that can provide their dimensions
type RectGetter interface {
	GetRect() (x, y, width, height int)
}

// GetHeightCategory returns the height category for a given terminal height
func GetHeightCategory(height int) int {
	switch {
	case height < MinTerminalHeight:
		return HeightCategoryTooSmall
	case height <= 45:
		return HeightCategorySmall
	case height <= 50:
		return HeightCategoryMedium
	case height < 65:
		return HeightCategoryLarge
	default:
		return HeightCategoryExtraLarge
	}
}

// GetTerminalHeight returns the terminal height from a primitive's GetRect
// Returns 50 (medium default) if the primitive is nil or not yet rendered
func GetTerminalHeight(root RectGetter) int {
	if root == nil {
		return 50 // Default to medium
	}
	_, _, _, height := root.GetRect()
	if height <= 0 {
		return 50 // Default if not yet rendered
	}
	return height
}

// IsBelowMinHeight returns true if the terminal height is below minimum
func IsBelowMinHeight(height int) bool {
	return height < MinTerminalHeight
}
