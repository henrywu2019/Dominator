package resourcepool

func newPool(max uint, metricsSubDirname string) *Pool {
	pool := &Pool{
		max:       max,
		semaphore: make(chan struct{}, max),
		unused:    make(map[*Resource]struct{}),
	}
	pool.registerMetrics(metricsSubDirname)
	return pool
}

func (pool *Pool) create(allocateReleaser AllocateReleaser) *Resource {
	return &Resource{
		pool:             pool,
		allocateReleaser: allocateReleaser,
		semaphore:        make(chan struct{}, 1)}
}

func (pool *Pool) getSlot(cancelChannel <-chan struct{}) bool {
	// Grab a slot (the right to have a resource in use).
	select {
	case pool.semaphore <- struct{}{}:
		return true
	default:
	}
	select {
	case pool.semaphore <- struct{}{}:
		return true
	case <-cancelChannel:
		return false
	}
}

func (resource *Resource) get(cancelChannel <-chan struct{}) error {
	drainSemaphore := false
	defer func() {
		if drainSemaphore {
			<-resource.semaphore
		}
	}()
	select {
	case resource.semaphore <- struct{}{}:
		drainSemaphore = true
	default:
		select {
		case resource.semaphore <- struct{}{}:
			drainSemaphore = true
		case <-cancelChannel:
			return ErrorPutTimeout
		}
	}
	pool := resource.pool
	if !pool.getSlot(cancelChannel) {
		return ErrorResourceLimitExceeded
	}
	pool.lock.Lock()
	defer pool.lock.Unlock()
	if resource.allocated {
		delete(pool.unused, resource)
		pool.numUnused = uint(len(pool.unused))
		drainSemaphore = false
		pool.numUsed++
		return nil
	}
	if pool.numUsed+uint(len(pool.unused))+pool.numReleasing >= pool.max {
		// Need to grab a free resource and release. Be lazy: do a random pick.
		var resourceToRelease *Resource
		for res := range pool.unused {
			resourceToRelease = res
			break
		}
		if resourceToRelease == nil {
			panic("No free resource to release")
		}
		if !resourceToRelease.allocated {
			panic("Resource is not allocated")
		}
		delete(pool.unused, resourceToRelease)
		pool.numUnused = uint(len(pool.unused))
		resourceToRelease.allocated = false
		pool.numReleasing++
		resourceToRelease.releasing.Lock()
		pool.lock.Unlock()
		resourceToRelease.releaseError =
			resourceToRelease.allocateReleaser.Release()
		pool.lock.Lock()
		resourceToRelease.releasing.Unlock()
		pool.numReleasing--
	}
	resource.allocating = true
	resource.allocated = true
	pool.numUsed++
	pool.lock.Unlock()
	resource.releasing.Lock() // Wait for myself to finish releasing.
	resource.releasing.Unlock()
	err := resource.allocateReleaser.Allocate()
	pool.lock.Lock()
	resource.allocating = false
	if err != nil {
		resource.allocated = false
		pool.numUsed--
		<-pool.semaphore // Free up a slot for someone else.
		return err
	}
	drainSemaphore = false
	return nil
}

func (resource *Resource) put() {
	pool := resource.pool
	pool.lock.Lock()
	if !resource.allocated {
		pool.lock.Unlock()
		return
	}
	if len(resource.semaphore) < 1 {
		pool.lock.Unlock()
		panic("Resource was not gotten")
	}
	if resource.releaseOnPut {
		resource.release(true)
		return
	}
	pool.unused[resource] = struct{}{}
	pool.numUnused = uint(len(pool.unused))
	<-resource.semaphore
	pool.numUsed--
	pool.lock.Unlock()
	<-pool.semaphore // Free up a slot for someone else.
}

func (resource *Resource) release(haveLock bool) error {
	pool := resource.pool
	if !haveLock {
		pool.lock.Lock()
	}
	if resource.allocating {
		pool.lock.Unlock()
		panic("Resource is allocating")
	}
	if !resource.allocated {
		pool.lock.Unlock()
		return resource.releaseError
	}
	delete(resource.pool.unused, resource)
	pool.numUnused = uint(len(pool.unused))
	resource.allocated = false
	wasUsed := false
	if len(resource.semaphore) > 0 {
		wasUsed = true
		<-resource.semaphore
		pool.numUsed--
	}
	pool.numReleasing++
	pool.lock.Unlock()
	resource.releaseError = resource.allocateReleaser.Release()
	pool.lock.Lock()
	pool.numReleasing--
	pool.lock.Unlock()
	if wasUsed {
		<-pool.semaphore // Free up a slot for someone else.
	}
	return resource.releaseError
}

func (resource *Resource) scheduleRelease() error {
	resource.pool.lock.Lock()
	if len(resource.semaphore) > 0 {
		resource.releaseOnPut = true
		resource.pool.lock.Unlock()
		return nil
	}
	return resource.release(true)
}
