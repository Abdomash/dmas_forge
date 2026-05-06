package specs

import (
	"github.com/blueprint-uservices/blueprint/blueprint/pkg/wiring"
	"github.com/blueprint-uservices/blueprint/plugins/mongodb"
	"github.com/blueprint-uservices/blueprint/plugins/workflow"
	"github.com/vaastav/agentic_blueprint/ai_plugins/model"
	"github.com/vaastav/agentic_blueprint/ai_plugins/openai_plugin"
	"github.com/vaastav/agentic_blueprint/ai_plugins/rag_plugin"
	ragruntime "github.com/vaastav/agentic_blueprint/ai_runtime/plugins/rag"
	"github.com/vaastav/agentic_blueprint/ai_runtime/plugins/vectorstore"
	wf "github.com/vaastav/agentic_blueprint/examples/agentic-hotel/workflow"
)

type hotelServices struct {
	geo         string
	rate        string
	profile     string
	reservation string
	search      string
	advisor     string
	support     string
	frontend    string
	dbs         []string
}

func defineHotelServices(spec wiring.WiringSpec) (hotelServices, error) {
	m, err := model.GetModelInfo()
	if err != nil {
		return hotelServices{}, err
	}

	geoDB := mongodb.Container(spec, "geo_db")
	rateDB := mongodb.Container(spec, "rate_db")
	profileDB := mongodb.Container(spec, "profile_db")
	reservationDB := mongodb.Container(spec, "reservation_db")

	advisorCore := openai_plugin.OpenAILLMAgent(
		spec,
		"advisor_core",
		m.URL,
		m.Key,
		m.Name,
		openai_plugin.AgentConfig{},
	)
	supportBase := openai_plugin.OpenAILLMAgent(
		spec,
		"support_base",
		m.URL,
		m.Key,
		m.Name,
		openai_plugin.AgentConfig{},
	)

	vectorStore := rag_plugin.VectorStore[*vectorstore.InMemoryVectorStore](spec, "support_vector_store")
	kb := rag_plugin.OpenAIKnowledgeBase(spec, "support_knowledge_base", m.URL, m.Key, m.EmbeddingModel, vectorStore)
	rag := rag_plugin.RAGAgent(spec, "support_rag_agent", supportBase, kb, ragruntime.RAGAgentConfig{
		ToolExposure: ragruntime.NoTools,
		AutoQuery:    true,
		TopK:         3,
	})

	s := hotelServices{dbs: []string{geoDB, rateDB, profileDB, reservationDB}}
	s.geo = workflow.Service[wf.GeoService](spec, "geo_service", geoDB)
	s.rate = workflow.Service[wf.RateService](spec, "rate_service", rateDB)
	s.profile = workflow.Service[wf.ProfileService](spec, "profile_service", profileDB)
	s.reservation = workflow.Service[wf.ReservationService](spec, "reservation_service", reservationDB)
	s.search = workflow.Service[wf.SearchService](spec, "search_service", s.geo, s.rate)
	s.advisor = workflow.Service[wf.HotelAdvisorAgent](
		spec,
		"hotel_advisor_agent",
		advisorCore,
		s.search,
		s.rate,
		s.profile,
		s.reservation,
	)
	s.support = workflow.Service[wf.SupportAgent](spec, "support_agent", rag, kb, s.reservation)
	s.frontend = workflow.Service[wf.FrontendService](
		spec,
		"frontend_service",
		s.search,
		s.profile,
		s.reservation,
		s.advisor,
		s.support,
	)

	return s, nil
}

func (s hotelServices) all() []string {
	return []string{s.geo, s.rate, s.profile, s.reservation, s.search, s.advisor, s.support, s.frontend}
}

func (s hotelServices) internal() []string {
	return []string{s.geo, s.rate, s.profile, s.reservation, s.search, s.advisor, s.support}
}
