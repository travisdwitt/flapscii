package main

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/nsf/termbox-go"
)

const (
	width  = 80
	height = 24
	birdX  = 10
)

type Bird struct {
	Y        float64
	Velocity float64
}

type Pipe struct {
	X         int
	GapTop    int
	GapSize   int
	TouchedYs map[int]bool // Track which Y positions of this pipe have been touched
}

type Particle struct {
	X          float64
	Y          float64
	VelX       float64
	VelY       float64
	Life       int
	Char       rune              // Character to display ('.' or '*')
	IsConfetti bool              // If true, this is confetti and won't mark things red
	Color      termbox.Attribute // Color for confetti particles
}

type BirdPiece struct {
	X        float64
	Y        float64
	VelX     float64
	VelY     float64
	Char     rune
	OnGround bool
	Bounces  int
	Color    termbox.Attribute // Color of the piece (red or light gray)
}

type Game struct {
	Bird            Bird
	Pipes           []Pipe
	Particles       []Particle
	Pieces          []BirdPiece
	TouchedCells    map[string]bool // Track touched cells: "x,y" -> true
	Score           int
	LastCheckpoint  int // Track last checkpoint to create confetti only once
	Dying           bool
	GameOver        bool
	Frame           int
	ScrollSpeed     float64 // Current scroll speed (decreases when bird dies)
	BaseScrollSpeed float64 // Base scroll speed (increases with checkpoints)
	FlashFrames     int     // Frames remaining for red screen flash
	BawkFrames      int     // Frames remaining to show "*BAWK*" message
	GroundPoofs     int     // Number of poofs created from ground pieces
	WindowX         int     // X offset for centered game window
	WindowY         int     // Y offset for centered game window
}

func main() {
	err := termbox.Init()
	if err != nil {
		panic(err)
	}
	defer termbox.Close()

	termbox.SetInputMode(termbox.InputEsc)

	rand.Seed(time.Now().UnixNano())

	// Get terminal size and calculate centered window position
	termWidth, termHeight := termbox.Size()
	game := NewGame()
	game.WindowX = (termWidth - width - 2) / 2   // Center horizontally (accounting for border)
	game.WindowY = (termHeight - height - 2) / 2 // Center vertically (accounting for border)

	eventQueue := make(chan termbox.Event)
	go func() {
		for {
			eventQueue <- termbox.PollEvent()
		}
	}()

	ticker := time.NewTicker(50 * time.Millisecond) // ~20 FPS
	defer ticker.Stop()

	for {
		select {
		case ev := <-eventQueue:
			if ev.Type == termbox.EventKey {
				if ev.Key == termbox.KeyEsc || ev.Key == termbox.KeyCtrlC {
					return
				}
				if ev.Ch == ' ' || ev.Key == termbox.KeySpace {
					if game.GameOver {
						game = NewGame()
						// Recalculate window position for centered display
						termWidth, termHeight := termbox.Size()
						game.WindowX = (termWidth - width - 2) / 2
						game.WindowY = (termHeight - height - 2) / 2
					} else if !game.Dying {
						// Only allow flapping if not dying
						game.Bird.Velocity = -1.2 // Jump - reduced for better control
					}
					// If dying, ignore space presses - bird must fall
				}
			}
		case <-ticker.C:
			// Continue updating if game is active, or if there are still pieces/particles animating
			hasActiveAnimation := len(game.Pieces) > 0 || len(game.Particles) > 0
			if !game.GameOver || game.Dying || hasActiveAnimation {
				game.Update()
			}
			game.Render()
		}
	}
}

func NewGame() *Game {
	return &Game{
		Bird: Bird{
			Y:        float64(height / 2),
			Velocity: 0,
		},
		Pipes:           []Pipe{},
		Particles:       []Particle{},
		Pieces:          []BirdPiece{},
		TouchedCells:    make(map[string]bool),
		Score:           0,
		LastCheckpoint:  -1,
		Dying:           false,
		GameOver:        false,
		Frame:           0,
		ScrollSpeed:     1.0, // Start at full speed
		BaseScrollSpeed: 1.0, // Base speed increases with checkpoints
		FlashFrames:     0,   // No flash initially
		BawkFrames:      0,   // No bawk message initially
		GroundPoofs:     0,   // No ground poofs initially
		WindowX:         0,   // Will be set in main()
		WindowY:         0,   // Will be set in main()
	}
}

func (g *Game) createPoof(x, y float64) {
	// Create multiple particles in random directions
	for i := 0; i < 20; i++ {
		speed := 0.3 + rand.Float64()*0.5 // Random speed between 0.3 and 0.8
		// Randomly assign '*' to about 25% of particles for variety
		char := '.'
		if rand.Float64() < 0.25 {
			char = '*'
		}
		g.Particles = append(g.Particles, Particle{
			X:          x,
			Y:          y,
			VelX:       speed * (rand.Float64()*2 - 1), // Random X velocity
			VelY:       speed * (rand.Float64()*2 - 1), // Random Y velocity
			Life:       20,                             // Particle lifetime
			Char:       char,
			IsConfetti: false,
			Color:      termbox.ColorRed,
		})
	}
}

func (g *Game) createTrail(x, y float64) {
	// Create trail particles that follow a piece as it falls
	// Create 2-3 particles per frame for a continuous trail
	for i := 0; i < 3; i++ {
		g.Particles = append(g.Particles, Particle{
			X:          x + (rand.Float64()*0.5 - 0.25), // Slight random offset
			Y:          y,
			VelX:       (rand.Float64()*0.2 - 0.1), // Small horizontal drift
			VelY:       -0.1 + rand.Float64()*0.1,  // Slight upward velocity to trail behind
			Life:       10,                         // Shorter lifetime for trail effect
			Char:       '.',                        // Trail particles are always dots
			IsConfetti: false,
			Color:      termbox.ColorRed,
		})
	}
}

func (g *Game) createConfetti(x, y float64) {
	// Create colorful confetti particles poofing from the bird's position
	colors := []termbox.Attribute{
		termbox.ColorRed,
		termbox.ColorGreen,
		termbox.ColorYellow,
		termbox.ColorBlue,
		termbox.ColorMagenta,
		termbox.ColorCyan,
	}

	// Create confetti poofing outward from bird position
	for i := 0; i < 30; i++ {
		speed := 0.4 + rand.Float64()*0.6 // Random speed between 0.4 and 1.0
		color := colors[rand.Intn(len(colors))]

		g.Particles = append(g.Particles, Particle{
			X:          x,
			Y:          y,
			VelX:       speed * (rand.Float64()*2 - 1), // Random X velocity in all directions
			VelY:       speed * (rand.Float64()*2 - 1), // Random Y velocity in all directions
			Life:       60,                             // Longer lifetime for confetti
			Char:       '*',
			IsConfetti: true,
			Color:      color,
		})
	}
}

func (g *Game) randomPieceColor() termbox.Attribute {
	// 50% chance of red or light gray
	if rand.Float64() < 0.5 {
		return termbox.ColorRed
	}
	return termbox.ColorWhite // Light gray
}

func (g *Game) breakBirdIntoPieces() {
	// Break the bird into pieces: (, o, >
	birdXPos := float64(birdX)
	birdYPos := g.Bird.Y

	// Create pieces with different velocities and random colors (50% red, 50% light gray)
	g.Pieces = []BirdPiece{
		{
			X:        birdXPos,
			Y:        birdYPos,
			VelX:     -0.3 + rand.Float64()*0.2, // Leftward drift
			VelY:     g.Bird.Velocity + rand.Float64()*0.3,
			Char:     '(',
			OnGround: false,
			Bounces:  0,
			Color:    g.randomPieceColor(),
		},
		{
			X:        birdXPos + 1,
			Y:        birdYPos,
			VelX:     rand.Float64()*0.4 - 0.2, // Random horizontal
			VelY:     g.Bird.Velocity + rand.Float64()*0.3,
			Char:     'x',
			OnGround: false,
			Bounces:  0,
			Color:    g.randomPieceColor(),
		},
		{
			X:        birdXPos + 2,
			Y:        birdYPos,
			VelX:     0.3 + rand.Float64()*0.2, // Rightward drift
			VelY:     g.Bird.Velocity + rand.Float64()*0.3,
			Char:     '>',
			OnGround: false,
			Bounces:  0,
			Color:    g.randomPieceColor(),
		},
	}
}

func (g *Game) Update() {
	g.Frame++

	// Decrement flash frames
	if g.FlashFrames > 0 {
		g.FlashFrames--
	}

	// Decrement bawk frames
	if g.BawkFrames > 0 {
		g.BawkFrames--
	}

	// Increase base scroll speed after every pipe passed
	if !g.Dying && !g.GameOver {
		// Increase by 0.1 per pipe passed
		g.BaseScrollSpeed = 1.0 + float64(g.Score)*0.1
		g.ScrollSpeed = g.BaseScrollSpeed

		// Create confetti when passing a checkpoint (every 10 pipes, only once per checkpoint)
		checkpoint := g.Score / 10
		if g.Score > 0 && g.Score%10 == 0 && checkpoint > g.LastCheckpoint {
			// Confetti poofs from the bird's position
			g.createConfetti(float64(birdX+1), g.Bird.Y)
			g.LastCheckpoint = checkpoint
		}
	}

	// Decrease scroll speed when dying (momentum loss)
	if g.Dying || g.GameOver {
		if g.ScrollSpeed > 0 {
			g.ScrollSpeed -= 0.08 // Stop momentum quicker
			if g.ScrollSpeed < 0 {
				g.ScrollSpeed = 0
			}
		}
	}

	// Update particles
	for i := len(g.Particles) - 1; i >= 0; i-- {
		// Apply gravity to particles
		g.Particles[i].VelY += 0.08
		g.Particles[i].X += g.Particles[i].VelX
		g.Particles[i].Y += g.Particles[i].VelY

		// Check if particle hit the ground
		particleX := int(g.Particles[i].X)
		particleY := int(g.Particles[i].Y)
		if particleY >= height-1 {
			// Mark ground cell as touched (only if not confetti)
			if !g.Particles[i].IsConfetti && particleX >= 0 && particleX < width {
				key := fmt.Sprintf("ground:%d,%d", particleX, height-1)
				g.TouchedCells[key] = true
			}
			// Remove particle when it hits ground
			g.Particles = append(g.Particles[:i], g.Particles[i+1:]...)
			continue
		}

		g.Particles[i].Life--
		if g.Particles[i].Life <= 0 {
			g.Particles = append(g.Particles[:i], g.Particles[i+1:]...)
		}
	}

	// Update bird pieces if they exist
	if len(g.Pieces) > 0 {
		allOnGround := true
		for i := range g.Pieces {
			if !g.Pieces[i].OnGround {
				// Update piece physics
				g.Pieces[i].VelY += 0.12 // Gravity
				g.Pieces[i].X += g.Pieces[i].VelX
				g.Pieces[i].Y += g.Pieces[i].VelY

				// Create trail for each piece
				g.createTrail(g.Pieces[i].X, g.Pieces[i].Y)

				// Check for pipe collisions
				pieceX := int(g.Pieces[i].X)
				pieceY := int(g.Pieces[i].Y)
				for _, pipe := range g.Pipes {
					// Check if piece is at pipe X position (pipe spans X-1 and X)
					if pieceX == pipe.X || pieceX == pipe.X-1 {
						// Check if piece is in a pipe segment (not in the gap)
						if pieceY < pipe.GapTop || pieceY >= pipe.GapTop+pipe.GapSize {
							// Mark the specific pipe cell that was touched
							key := fmt.Sprintf("pipe:%d,%d", pieceX, pieceY)
							g.TouchedCells[key] = true
							// Bounce the piece
							if g.Pieces[i].Bounces < 2 {
								g.Pieces[i].VelY = -g.Pieces[i].VelY * 0.6 // Bounce with reduced energy
								g.Pieces[i].VelX *= 0.8                    // Reduce horizontal velocity
								g.Pieces[i].Bounces++
								g.Pieces[i].Y = float64(pieceY - 1) // Move piece up slightly
							} else {
								g.Pieces[i].OnGround = true
							}
						}
					}
				}

				// Check if piece hit the ground
				if g.Pieces[i].Y >= float64(height-2) {
					// Mark ground cell as touched
					groundX := int(g.Pieces[i].X)
					if groundX >= 0 && groundX < width {
						key := fmt.Sprintf("ground:%d,%d", groundX, height-1)
						g.TouchedCells[key] = true
					}

					// Bounce if bounces remaining
					if g.Pieces[i].Bounces < 2 {
						g.Pieces[i].Y = float64(height - 2)
						g.Pieces[i].VelY = -g.Pieces[i].VelY * 0.6 // Bounce with reduced energy
						g.Pieces[i].VelX *= 0.8                    // Reduce horizontal velocity
						g.Pieces[i].Bounces++
					} else {
						// No more bounces, stop on ground
						g.Pieces[i].Y = float64(height - 2)
						g.Pieces[i].VelY = 0
						g.Pieces[i].VelX = 0
						g.Pieces[i].OnGround = true
					}
				}

				if !g.Pieces[i].OnGround {
					allOnGround = false
				}
			}
		}
		// If all pieces are on the ground, mark game over but continue animating
		if allOnGround && !g.GameOver {
			g.GameOver = true
			g.Dying = false
		}

		// Occasionally poof particles from random pieces on the ground when game is over (limit 5)
		if g.GameOver && len(g.Pieces) > 0 && g.Frame%30 == 0 && g.GroundPoofs < 5 {
			// Find pieces that are on the ground
			groundPieces := []int{}
			for i, piece := range g.Pieces {
				if piece.OnGround {
					groundPieces = append(groundPieces, i)
				}
			}
			// Pick a random piece on the ground and poof particles from it
			if len(groundPieces) > 0 {
				randomIndex := groundPieces[rand.Intn(len(groundPieces))]
				pieceIndex := randomIndex
				piece := g.Pieces[pieceIndex]
				g.createPoof(piece.X, piece.Y)
				// Turn the piece red if it isn't already
				if g.Pieces[pieceIndex].Color != termbox.ColorRed {
					g.Pieces[pieceIndex].Color = termbox.ColorRed
				}
				g.GroundPoofs++ // Increment poof counter
			}
		}
	}

	// Update bird physics (only if not broken into pieces)
	if len(g.Pieces) == 0 {
		if !g.GameOver && !g.Dying {
			g.Bird.Velocity += 0.12 // Gravity - slightly reduced for smoother control
			g.Bird.Y += g.Bird.Velocity
		} else if g.Dying {
			// Bird is dying, continue falling
			g.Bird.Velocity += 0.12
			g.Bird.Y += g.Bird.Velocity
			// Create trail particles as bird falls
			g.createTrail(float64(birdX+1), g.Bird.Y)
			// Check if bird hit the ground (ground is at height-1, stop at height-2)
			if g.Bird.Y >= float64(height-2) {
				g.Bird.Y = float64(height - 2)
				g.Bird.Velocity = 0
				g.GameOver = true
				g.Dying = false
			}
		}
	}

	// Check collisions with top/bottom
	if !g.GameOver && !g.Dying && len(g.Pieces) == 0 {
		if g.Bird.Y < 1 {
			g.createPoof(float64(birdX+1), g.Bird.Y)
			g.Bird.Y = 1
			g.breakBirdIntoPieces()
			g.Dying = true
			g.FlashFrames = 1 // Flash screen red
			g.BawkFrames = 3  // Show "*BAWK*" message
		}
		if g.Bird.Y >= float64(height-1) {
			g.createPoof(float64(birdX+1), float64(height-2))
			g.Bird.Y = float64(height - 2)
			g.breakBirdIntoPieces()
			g.Dying = true
			g.FlashFrames = 1 // Flash screen red
			g.BawkFrames = 3  // Show "*BAWK*" message
		}
	}

	// Generate new pipes (only if screen is still scrolling)
	if g.ScrollSpeed > 0 && g.Frame%60 == 0 {
		gapSize := 8
		gapTop := rand.Intn(height-gapSize-4) + 2
		g.Pipes = append(g.Pipes, Pipe{
			X:         width - 1,
			GapTop:    gapTop,
			GapSize:   gapSize,
			TouchedYs: make(map[int]bool),
		})
	}

	// Update pipes
	for i := len(g.Pipes) - 1; i >= 0; i-- {
		// Move pipe based on scroll speed
		// Use frame-based movement for smooth deceleration
		if g.ScrollSpeed >= 1.0 {
			// Full speed - move every frame
			g.Pipes[i].X--
		} else if g.ScrollSpeed > 0 {
			// Reduced speed - move every N frames (inverse of speed)
			// Higher speed = move more frequently
			framesPerMove := int(1.0 / g.ScrollSpeed)
			if framesPerMove < 1 {
				framesPerMove = 1
			}
			if g.Frame%framesPerMove == 0 {
				g.Pipes[i].X--
			}
		}
		// When scroll speed reaches 0, pipes stop moving

		// Remove pipes that are off screen
		if g.Pipes[i].X < -5 {
			g.Pipes = append(g.Pipes[:i], g.Pipes[i+1:]...)
			if !g.Dying && !g.GameOver {
				g.Score++
			}
			continue
		}

		// Check if pieces pass through pipe segments
		for _, piece := range g.Pieces {
			pieceX := int(piece.X)
			pieceY := int(piece.Y)
			// Check if piece is at pipe X position (pipe spans X-1 and X)
			if (pieceX == g.Pipes[i].X || pieceX == g.Pipes[i].X-1) && pieceY >= 0 && pieceY < height {
				// Check if piece is in a pipe segment (not in gap)
				if pieceY < g.Pipes[i].GapTop || pieceY >= g.Pipes[i].GapTop+g.Pipes[i].GapSize {
					// Mark this Y position of this pipe as touched (stays red as pipe moves)
					g.Pipes[i].TouchedYs[pieceY] = true
					// Also mark screen position for rendering
					key := fmt.Sprintf("pipe:%d,%d", pieceX, pieceY)
					g.TouchedCells[key] = true
				}
			}
		}

		// Check if particles pass through pipe segments (skip confetti)
		for _, particle := range g.Particles {
			if particle.IsConfetti {
				continue // Confetti doesn't mark things red
			}
			particleX := int(particle.X)
			particleY := int(particle.Y)
			// Check if particle is at pipe X position (pipe spans X-1 and X)
			if (particleX == g.Pipes[i].X || particleX == g.Pipes[i].X-1) && particleY >= 0 && particleY < height {
				// Check if particle is in a pipe segment (not in gap)
				if particleY < g.Pipes[i].GapTop || particleY >= g.Pipes[i].GapTop+g.Pipes[i].GapSize {
					// Mark this Y position of this pipe as touched (stays red as pipe moves)
					g.Pipes[i].TouchedYs[particleY] = true
					// Also mark screen position for rendering
					key := fmt.Sprintf("pipe:%d,%d", particleX, particleY)
					g.TouchedCells[key] = true
				}
			}
		}

		// Check collision
		if !g.GameOver && !g.Dying && len(g.Pieces) == 0 && g.checkCollision(g.Pipes[i]) {
			g.createPoof(float64(birdX+1), g.Bird.Y)
			g.breakBirdIntoPieces()
			g.Dying = true
			g.FlashFrames = 1 // Flash screen red
			g.BawkFrames = 3  // Show "*BAWK*" message
		}
	}
}

func (g *Game) checkCollision(pipe Pipe) bool {
	birdY := int(g.Bird.Y)

	// Check if bird is at pipe's X position (bird spans 3 cells: (o>)
	if (birdX >= pipe.X-1 && birdX <= pipe.X+1) ||
		(birdX+1 >= pipe.X-1 && birdX+1 <= pipe.X+1) ||
		(birdX+2 >= pipe.X-1 && birdX+2 <= pipe.X+1) {
		// Check if bird is in the gap
		if birdY < pipe.GapTop || birdY >= pipe.GapTop+pipe.GapSize {
			return true
		}
		// Also check if bird hits the top or bottom of the gap
		if birdY == pipe.GapTop-1 || birdY == pipe.GapTop+pipe.GapSize {
			return true
		}
	}
	return false
}

func (g *Game) setCell(x, y int, ch rune, fg, bg termbox.Attribute) {
	// Offset by window position + 1 for border
	screenX := g.WindowX + 1 + x
	screenY := g.WindowY + 1 + y
	termWidth, termHeight := termbox.Size()
	if screenX >= 0 && screenX < termWidth && screenY >= 0 && screenY < termHeight {
		termbox.SetCell(screenX, screenY, ch, fg, bg)
	}
}

func (g *Game) Render() {
	termbox.Clear(termbox.ColorDefault, termbox.ColorDefault)

	// Draw dark gray border around game window
	borderColor := termbox.ColorBlack // Dark gray/black for border
	borderX := g.WindowX
	borderY := g.WindowY
	borderWidth := width + 2
	borderHeight := height + 2
	termWidth, termHeight := termbox.Size()

	// Draw top and bottom borders
	for x := 0; x < borderWidth; x++ {
		if borderX+x >= 0 && borderX+x < termWidth {
			// Top border
			if borderY >= 0 && borderY < termHeight {
				termbox.SetCell(borderX+x, borderY, '─', borderColor, termbox.ColorDefault)
			}
			// Bottom border
			if borderY+borderHeight-1 >= 0 && borderY+borderHeight-1 < termHeight {
				termbox.SetCell(borderX+x, borderY+borderHeight-1, '─', borderColor, termbox.ColorDefault)
			}
		}
	}

	// Draw left and right borders
	for y := 0; y < borderHeight; y++ {
		if borderY+y >= 0 && borderY+y < termHeight {
			// Left border
			if borderX >= 0 && borderX < termWidth {
				termbox.SetCell(borderX, borderY+y, '│', borderColor, termbox.ColorDefault)
			}
			// Right border
			if borderX+borderWidth-1 >= 0 && borderX+borderWidth-1 < termWidth {
				termbox.SetCell(borderX+borderWidth-1, borderY+y, '│', borderColor, termbox.ColorDefault)
			}
		}
	}

	// Draw corners
	if borderX >= 0 && borderX < termWidth && borderY >= 0 && borderY < termHeight {
		termbox.SetCell(borderX, borderY, '┌', borderColor, termbox.ColorDefault) // Top-left
	}
	if borderX+borderWidth-1 >= 0 && borderX+borderWidth-1 < termWidth && borderY >= 0 && borderY < termHeight {
		termbox.SetCell(borderX+borderWidth-1, borderY, '┐', borderColor, termbox.ColorDefault) // Top-right
	}
	if borderX >= 0 && borderX < termWidth && borderY+borderHeight-1 >= 0 && borderY+borderHeight-1 < termHeight {
		termbox.SetCell(borderX, borderY+borderHeight-1, '└', borderColor, termbox.ColorDefault) // Bottom-left
	}
	if borderX+borderWidth-1 >= 0 && borderX+borderWidth-1 < termWidth && borderY+borderHeight-1 >= 0 && borderY+borderHeight-1 < termHeight {
		termbox.SetCell(borderX+borderWidth-1, borderY+borderHeight-1, '┘', borderColor, termbox.ColorDefault) // Bottom-right
	}

	// Draw particles (poof effect and confetti)
	for _, p := range g.Particles {
		px := int(p.X)
		py := int(p.Y)
		if px >= 0 && px < width && py >= 0 && py < height {
			char := p.Char
			if char == 0 {
				char = '.' // Default to '.' if not set
			}
			color := p.Color
			if color == 0 {
				color = termbox.ColorRed // Default to red if not set
			}
			g.setCell(px, py, char, color, termbox.ColorDefault)
		}
	}

	// Draw pipes
	for _, pipe := range g.Pipes {
		for y := 0; y < height; y++ {
			if y < pipe.GapTop || y >= pipe.GapTop+pipe.GapSize {
				// Check if this Y position of this pipe was touched
				color := termbox.ColorGreen
				if pipe.TouchedYs[y] {
					color = termbox.ColorRed
				}

				if pipe.X >= 0 && pipe.X < width {
					g.setCell(pipe.X, y, '█', color, termbox.ColorDefault)
				}
				if pipe.X-1 >= 0 && pipe.X-1 < width {
					g.setCell(pipe.X-1, y, '█', color, termbox.ColorDefault)
				}
			}
		}
	}

	// Draw bird pieces if broken apart, otherwise draw normal bird
	if len(g.Pieces) > 0 {
		// Draw each piece
		for _, piece := range g.Pieces {
			px := int(piece.X)
			py := int(piece.Y)
			if px >= 0 && px < width && py >= 0 && py < height {
				g.setCell(px, py, piece.Char, piece.Color, termbox.ColorDefault)
			}
		}
		// Show "*BAWK*" message near the first piece when bird dies
		if g.BawkFrames > 0 && len(g.Pieces) > 0 {
			firstPiece := g.Pieces[0]
			bawkMsg := "*BAWK*"
			bawkX := int(firstPiece.X) + 4
			bawkY := int(firstPiece.Y)
			if bawkX+len(bawkMsg) < width && bawkY >= 0 && bawkY < height {
				for i, r := range bawkMsg {
					if bawkX+i < width {
						g.setCell(bawkX+i, bawkY, r, termbox.ColorCyan, termbox.ColorDefault)
					}
				}
			}
		}
	} else {
		// Draw bird
		birdY := int(g.Bird.Y)
		if birdY >= 0 && birdY < height {
			// Draw bird body: (o> or (x> when dying
			birdColor := termbox.ColorWhite // Light gray color
			birdChar := 'o'
			if g.Dying || g.GameOver {
				birdChar = 'x'
				birdColor = termbox.ColorRed
			}

			g.setCell(birdX, birdY, '(', birdColor, termbox.ColorDefault)
			if birdX+1 < width {
				g.setCell(birdX+1, birdY, birdChar, birdColor, termbox.ColorDefault)
			}
			if birdX+2 < width {
				g.setCell(birdX+2, birdY, '>', birdColor, termbox.ColorDefault)
			}
			// Draw wing animation based on frame (only when not dying)
			if !g.Dying && !g.GameOver {
				if g.Frame%8 < 4 && birdY-1 >= 0 && birdX+1 < width {
					g.setCell(birdX+1, birdY-1, '^', termbox.ColorWhite, termbox.ColorDefault)
				} else if birdY+1 < height && birdX+1 < width {
					g.setCell(birdX+1, birdY+1, 'v', termbox.ColorWhite, termbox.ColorDefault)
				}
			}

			// Draw "*BAWK*" message next to bird when it dies
			if g.BawkFrames > 0 {
				bawkMsg := "*BAWK*"
				bawkX := birdX + 4 // Position to the right of the bird
				if bawkX+len(bawkMsg) < width {
					for i, r := range bawkMsg {
						if bawkX+i < width {
							g.setCell(bawkX+i, birdY, r, termbox.ColorCyan, termbox.ColorDefault)
						}
					}
				}
			}
		}
	}

	// Draw ground
	for x := 0; x < width; x++ {
		key := fmt.Sprintf("ground:%d,%d", x, height-1)
		color := termbox.ColorGreen
		if g.TouchedCells[key] {
			color = termbox.ColorRed
		}
		g.setCell(x, height-1, '═', color, termbox.ColorDefault)
	}

	// Draw score
	scoreStr := fmt.Sprintf("Score: %d", g.Score)
	for i, r := range scoreStr {
		if i < width {
			g.setCell(i, 0, r, termbox.ColorWhite, termbox.ColorDefault)
		}
	}

	// Draw game over message
	if g.GameOver {
		// Flash "YOU DIED" in red
		msg := "YOU DIED"
		startX := (width - len(msg)) / 2
		// Flash effect: show red text, blinking every 20 frames
		youDiedColor := termbox.ColorRed
		if g.Frame%20 >= 10 {
			youDiedColor = termbox.ColorDefault // Invisible when not flashing
		}
		for i, r := range msg {
			if startX+i < width {
				g.setCell(startX+i, height/2-1, r, youDiedColor, termbox.ColorDefault)
			}
		}

		// "Press SPACE to restart" in white
		msg2 := "Press SPACE to restart"
		startX2 := (width - len(msg2)) / 2
		for i, r := range msg2 {
			if startX2+i < width {
				g.setCell(startX2+i, height/2, r, termbox.ColorWhite, termbox.ColorDefault)
			}
		}

		msg3 := "Press ESC to quit"
		startX3 := (width - len(msg3)) / 2
		for i, r := range msg3 {
			if startX3+i < width {
				g.setCell(startX3+i, height/2+1, r, termbox.ColorWhite, termbox.ColorDefault)
			}
		}
	} else {
		// Draw instructions
		msg := "Press SPACE to flap"
		for i, r := range msg {
			if i < width {
				g.setCell(i, height-2, r, termbox.ColorCyan, termbox.ColorDefault)
			}
		}
	}

	// Draw red flash overlay when bird dies (centered in game window)
	if g.FlashFrames > 0 {
		for x := 0; x < width; x++ {
			for y := 0; y < height; y++ {
				// Draw red background with transparent character for flash effect
				g.setCell(x, y, ' ', termbox.ColorDefault, termbox.ColorRed)
			}
		}
	}

	termbox.Flush()
}
