package plan

import (
	"fmt"
	"log"
	"plandex-server/db"
	"plandex-server/host"
	"plandex-server/model"
	"plandex-server/types"

	shared "plandex-shared"
)

func activatePlan(
	clients map[string]model.ClientInfo,
	plan *db.Plan,
	branch string,
	auth *types.ServerAuth,
	prompt string,
	buildOnly,
	autoContext bool,
	sessionId string,
) (*types.ActivePlan, error) {
	log.Printf("Activate plan: plan ID %s on branch %s\n", plan.Id, branch)

	// Check for an active model stream in the database first. This is needed
	// for multi-instance coordination (another server may own the stream).
	modelStream, err := db.GetActiveModelStream(plan.Id, branch)
	if err != nil {
		log.Printf("Error getting active model stream: %v\n", err)
		return nil, fmt.Errorf("error getting active model stream: %v", err)
	}

	if modelStream != nil {
		log.Printf("Tell: Active model stream found for plan ID %s on branch %s on host %s\n", plan.Id, branch, modelStream.InternalIp)
		return nil, fmt.Errorf("plan %s branch %s already has an active stream on host %s. Use 'plandex connect' to attach or 'plandex stop' to cancel", plan.Id, branch, modelStream.InternalIp)
	}

	// Atomically register the active plan. SetIfAbsent inside
	// CreateActivePlan ensures only one goroutine wins if two
	// concurrent tell/build requests race on the same plan+branch.
	active := CreateActivePlan(
		auth.OrgId,
		auth.User.Id,
		plan.Id,
		branch,
		prompt,
		buildOnly,
		autoContext,
		sessionId,
	)

	if active == nil {
		log.Printf("Tell: Active plan already registered for plan ID %s on branch %s\n", plan.Id, branch)
		return nil, fmt.Errorf("plan %s branch %s already has an active stream on this host. Use 'plandex connect' to attach or 'plandex stop' to cancel", plan.Id, branch)
	}

	modelStream = &db.ModelStream{
		OrgId:      auth.OrgId,
		PlanId:     plan.Id,
		InternalIp: host.Ip,
		Branch:     branch,
	}
	err = db.StoreModelStream(modelStream, active.Ctx, active.CancelFn)
	if err != nil {
		log.Printf("Tell: Error storing model stream for plan ID %s on branch %s: %v\n", plan.Id, branch, err)

		active.StreamDoneCh <- &shared.ApiError{Msg: fmt.Sprintf("Error storing model stream: %v", err)}

		return nil, fmt.Errorf("error storing model stream: %v", err)
	}

	active.ModelStreamId = modelStream.Id

	log.Printf("Tell: Model stream stored with ID %s for plan ID %s on branch %s\n", modelStream.Id, plan.Id, branch) // Log successful storage of model stream
	log.Println("Model stream id:", modelStream.Id)

	return active, nil
}
