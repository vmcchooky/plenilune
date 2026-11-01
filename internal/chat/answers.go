package chat

import (
	"fmt"
	"strings"

	"plenilune/internal/intent"
	"plenilune/internal/verify"
)

// The builders below assemble text strictly from the facts they emit:
// every number, status, packet and rule reference in a text comes from
// the artifact and appears in the fact list with its source field. The
// only other sentences are connective or suggestive ("ask about …"),
// which carry no factual claim about the analysis.

func (v *view) answerVerdict() (string, []Fact, string) {
	if v.comparison != nil {
		r := v.comparison.Report
		facts := []Fact{
			v.fact(v.s("Verdict", "Kết luận"), r.Verdict, "comparison.report.verdict"),
			v.fact(v.s("Search complete", "Tìm kiếm hoàn tất"), boolText(r.SearchComplete), "comparison.report.search_complete"),
		}
		if r.Reason != "" {
			facts = append(facts, v.fact(v.s("Stop reason", "Lý do dừng"), r.Reason, "comparison.report.reason"))
		}
		facts = append(facts,
			v.fact(v.s("Difference witnesses", "Witness khác biệt"), fmt.Sprint(len(r.Witnesses)), "comparison.report.witnesses"),
			v.fact(v.s("Obligations evaluated", "Điều kiện đã đánh giá"), fmt.Sprint(len(r.Obligations)), "comparison.report.obligations"),
		)
		var listing []string
		for i, o := range r.Obligations {
			facts = append(facts, v.fact(o.ID, o.Status, fmt.Sprintf("comparison.report.obligations[%d].status", i)))
			listing = append(listing, o.ID+" "+o.Status)
		}
		var b strings.Builder
		b.WriteString(v.s(fmt.Sprintf("The comparison verdict is %s. ", r.Verdict), fmt.Sprintf("Kết luận của comparison là %s. ", r.Verdict)))
		if r.SearchComplete {
			b.WriteString(v.s("The search completed within the modeled scope. ", "Tìm kiếm đã hoàn tất trong phạm vi mô hình. "))
		} else {
			b.WriteString(v.s(fmt.Sprintf("The search did not complete (stop reason: %s). ", r.Reason), fmt.Sprintf("Tìm kiếm chưa hoàn tất (lý do dừng: %s). ", r.Reason)))
		}
		b.WriteString(v.s(fmt.Sprintf("It records %d difference witness(es). ", len(r.Witnesses)), fmt.Sprintf("Artifact ghi %d witness khác biệt. ", len(r.Witnesses))))
		if len(listing) > 0 {
			b.WriteString(v.s("Obligations: "+strings.Join(listing, " · ")+".", "Điều kiện: "+strings.Join(listing, " · ")+"."))
		} else {
			b.WriteString(v.s("No obligations were evaluated in this comparison.", "Comparison này không đánh giá điều kiện nào."))
		}
		return ClassVerdict, capFacts(facts), b.String()
	}
	// Analysis only: there is deliberately no configuration-level
	// verdict; the per-object verdicts are the facts.
	objects := v.aclObjects()
	facts := []Fact{
		v.fact(v.s("Objects analyzed", "Đối tượng đã phân tích"), fmt.Sprint(v.analysis.Summary.Objects), "analysis.summary.objects"),
		v.fact(v.s("Changed objects", "Đối tượng thay đổi"), fmt.Sprint(v.analysis.Summary.Changed), "analysis.summary.changed"),
		v.fact(v.s("Equivalent objects", "Đối tượng tương đương"), fmt.Sprint(v.analysis.Summary.Equivalent), "analysis.summary.equivalent"),
		v.fact(v.s("Inconclusive objects", "Đối tượng chưa kết luận"), fmt.Sprint(v.analysis.Summary.Inconclusive), "analysis.summary.inconclusive"),
	}
	var parts []string
	for _, o := range objects {
		facts = append(facts, v.fact("ACL "+o.obj.Name, o.obj.ACL.Verdict, fmt.Sprintf("analysis.objects[%d].acl.verdict", o.index)))
		parts = append(parts, o.obj.Name+": "+o.obj.ACL.Verdict)
	}
	text := v.s("The analysis record has no single configuration verdict by design; each object carries its own. ",
		"Bản ghi analysis cố ý không có một kết luận chung cho cả cấu hình; mỗi đối tượng mang kết luận riêng. ")
	if len(parts) > 0 {
		text += v.s("Object verdicts: ", "Kết luận theo đối tượng: ") + strings.Join(parts, " · ") + "."
	}
	return ClassVerdict, capFacts(facts), text
}

func (v *view) answerObligations() (string, []Fact, string) {
	if v.comparison == nil {
		return v.notInArtifact(
			v.s("obligation results", "kết quả điều kiện"),
			v.s("Obligations are evaluated and recorded in the comparison artifact; this is an analysis record, which carries per-object verdicts instead.",
				"Điều kiện được đánh giá và ghi trong artifact comparison; đây là bản ghi analysis, vốn mang kết luận theo từng đối tượng."))
	}
	r := v.comparison.Report
	if len(r.Obligations) == 0 {
		facts := []Fact{v.fact(v.s("Obligations evaluated", "Điều kiện đã đánh giá"), "0", "comparison.report.obligations")}
		return ClassObligations, facts, v.s(
			"No obligations were evaluated in this comparison, so there is no obligation outcome to report; the verdict is behavior-only.",
			"Comparison này không đánh giá điều kiện nào nên không có kết quả điều kiện để báo; kết luận chỉ dựa trên hành vi.")
	}
	var facts []Fact
	var listing []string
	for i, o := range r.Obligations {
		kind := ""
		if v.comparison.Conditions != nil && i < len(v.comparison.Conditions.Obligations) {
			kind = v.comparison.Conditions.Obligations[i].Kind
		}
		label := o.ID
		if kind != "" {
			label = o.ID + " (" + kind + ")"
		}
		facts = append(facts, v.fact(label, o.Status, fmt.Sprintf("comparison.report.obligations[%d].status", i)))
		listing = append(listing, o.ID+" "+o.Status)
		if o.Status == "VIOLATED" && o.Witness != nil {
			facts = append(facts, v.witnessFact(o.ID+" witness", *o.Witness, fmt.Sprintf("comparison.report.obligations[%d].witness", i)))
		}
	}
	text := v.s("Obligation results: ", "Kết quả điều kiện: ") + strings.Join(listing, " · ") + "."
	return ClassObligations, capFacts(facts), text
}

func (v *view) answerObligation(q questionInfo, wantsWitness bool) (string, []Fact, string) {
	if v.comparison == nil {
		return v.notInArtifact(
			v.s("obligation results", "kết quả điều kiện"),
			v.s("Obligations are evaluated and recorded in the comparison artifact; this is an analysis record, which carries per-object verdicts instead.",
				"Điều kiện được đánh giá và ghi trong artifact comparison; đây là bản ghi analysis, vốn mang kết luận theo từng đối tượng."))
	}
	r := v.comparison.Report
	idx := -1
	for i, o := range r.Obligations {
		if o.ID == q.obligationID {
			idx = i
			break
		}
	}
	if idx < 0 {
		var ids []string
		for _, o := range r.Obligations {
			ids = append(ids, o.ID)
		}
		facts := []Fact{v.fact(v.s("Evaluated obligations", "Điều kiện đã đánh giá"), strings.Join(ids, ", "), "comparison.report.obligations")}
		text := v.s(fmt.Sprintf("%s is not among the obligations evaluated in this artifact.", q.obligationID),
			fmt.Sprintf("%s không nằm trong các điều kiện đã đánh giá trong artifact này.", q.obligationID))
		if len(ids) > 0 {
			text += v.s(" Evaluated: "+strings.Join(ids, ", ")+".", " Đã đánh giá: "+strings.Join(ids, ", ")+".")
		}
		return ClassNotInArtifact, facts, text
	}
	o := r.Obligations[idx]
	base := fmt.Sprintf("comparison.report.obligations[%d]", idx)
	facts := []Fact{v.fact(o.ID+" status", o.Status, base+".status")}
	if v.comparison.Conditions != nil && idx < len(v.comparison.Conditions.Obligations) {
		def := v.comparison.Conditions.Obligations[idx]
		facts = append(facts, v.fact(o.ID+" ("+def.Kind+")", regionText(def.Match), fmt.Sprintf("comparison.conditions.obligations[%d].match", idx)))
	}
	if o.Reason != "" {
		facts = append(facts, v.fact(o.ID+" reason", o.Reason, base+".reason"))
	}
	class := ClassObligation
	var text string
	switch o.Status {
	case "VIOLATED":
		if o.Witness != nil {
			facts = append(facts,
				v.fact(o.ID+" witness packet", packetText(o.Witness.Packet), base+".witness.packet"),
				v.fact(o.ID+" witness before", v.decisionText(o.Witness.Before), base+".witness.before"),
				v.fact(o.ID+" witness after", v.decisionText(o.Witness.After), base+".witness.after"),
			)
		}
		if wantsWitness {
			class = ClassObligationWitness
		}
		text = v.s(fmt.Sprintf("%s is VIOLATED. The witness packet %s is decided %s before and %s after, which breaks the obligation.",
			o.ID, packetText(o.Witness.Packet), v.decisionText(o.Witness.Before), v.decisionText(o.Witness.After)),
			fmt.Sprintf("%s bị VIOLATED. Gói witness %s được quyết định %s ở bản trước và %s ở bản sau, phá vỡ điều kiện.",
				o.ID, packetText(o.Witness.Packet), v.decisionText(o.Witness.Before), v.decisionText(o.Witness.After)))
	case "HOLDS":
		text = v.s(fmt.Sprintf("%s HOLDS within the modeled scope. A holding obligation carries no witness — witnesses are recorded for violations.", o.ID),
			fmt.Sprintf("%s HOLDS trong phạm vi mô hình. Điều kiện đạt không mang witness — witness chỉ được ghi cho vi phạm.", o.ID))
	default: // INCONCLUSIVE
		text = v.s(fmt.Sprintf("%s is INCONCLUSIVE: the search stopped before this obligation could be decided (reason: %s). Nothing about it is claimed either way.", o.ID, o.Reason),
			fmt.Sprintf("%s INCONCLUSIVE: tìm kiếm đã dừng trước khi quyết định được điều kiện này (lý do: %s). Không có khẳng định nào về nó theo cả hai phía.", o.ID, o.Reason))
	}
	return class, capFacts(facts), text
}

func (v *view) firstWitness() (verify.Witness, string, bool) {
	if v.comparison != nil {
		r := v.comparison.Report
		if len(r.Witnesses) > 0 {
			return r.Witnesses[0], "comparison.report.witnesses[0]", true
		}
		for i, o := range r.Obligations {
			if o.Witness != nil {
				return *o.Witness, fmt.Sprintf("comparison.report.obligations[%d].witness", i), true
			}
		}
	}
	if o, ok := v.pickObject(""); ok && len(o.obj.ACL.Witnesses) > 0 {
		return o.obj.ACL.Witnesses[0], fmt.Sprintf("analysis.objects[%d].acl.witnesses[0]", o.index), true
	}
	return verify.Witness{}, "", false
}

func (v *view) answerWitnesses() (string, []Fact, string) {
	if v.comparison != nil {
		r := v.comparison.Report
		facts := []Fact{v.fact(v.s("Difference witnesses", "Witness khác biệt"), fmt.Sprint(len(r.Witnesses)), "comparison.report.witnesses")}
		if r.WitnessesTruncated {
			facts = append(facts, v.fact(v.s("Witness list truncated", "Danh sách witness bị cắt"), "true", "comparison.report.witnesses_truncated"))
		}
		if len(r.Witnesses) == 0 {
			return ClassWitnesses, facts, v.s(
				fmt.Sprintf("This artifact records no difference witness (verdict: %s). Witnesses are recorded only for proven differences and violated obligations.", r.Verdict),
				fmt.Sprintf("Artifact này không ghi witness khác biệt nào (kết luận: %s). Witness chỉ được ghi cho khác biệt đã chứng minh và điều kiện bị vi phạm.", r.Verdict))
		}
		var first string
		for i, w := range r.Witnesses {
			if i >= maxListedWitnesses {
				break
			}
			facts = append(facts, v.witnessFact(w.ID, w, fmt.Sprintf("comparison.report.witnesses[%d]", i)))
			if i == 0 {
				first = packetText(w.Packet)
			}
		}
		text := v.s(fmt.Sprintf("The artifact records %d difference witness(es); the first is %s. Each listed witness was re-checked by the independent evaluator.", len(r.Witnesses), first),
			fmt.Sprintf("Artifact ghi %d witness khác biệt; witness đầu tiên là %s. Mỗi witness liệt kê đã được evaluator độc lập kiểm lại.", len(r.Witnesses), first))
		return ClassWitnesses, capFacts(facts), text
	}
	if o, ok := v.pickObject(""); ok {
		ws := o.obj.ACL.Witnesses
		src := fmt.Sprintf("analysis.objects[%d].acl.witnesses", o.index)
		facts := []Fact{v.fact("ACL "+o.obj.Name+" witnesses", fmt.Sprint(len(ws)), src)}
		for i, w := range ws {
			if i >= maxListedWitnesses {
				break
			}
			facts = append(facts, v.witnessFact(w.ID, w, fmt.Sprintf("%s[%d]", src, i)))
		}
		text := v.s(fmt.Sprintf("ACL %s records %d witness(es) in the analysis record.", o.obj.Name, len(ws)),
			fmt.Sprintf("ACL %s ghi %d witness trong bản ghi analysis.", o.obj.Name, len(ws)))
		return ClassWitnesses, capFacts(facts), text
	}
	return v.notInArtifact(v.s("witnesses", "witness"),
		v.s("No analyzed ACL object with witnesses is present in this artifact.", "Không có đối tượng ACL đã phân tích kèm witness trong artifact này."))
}

func (v *view) answerDecidingRule() (string, []Fact, string) {
	w, src, ok := v.firstWitness()
	if !ok {
		return v.notInArtifact(v.s("a deciding rule", "rule quyết định"),
			v.s("No witness is recorded in this artifact, so no deciding rule can be named from it.",
				"Artifact này không ghi witness nào nên không thể nêu rule quyết định từ nó."))
	}
	facts := []Fact{
		v.fact(v.s("Witness packet", "Gói witness"), packetText(w.Packet), src+".packet"),
		v.fact(v.s("Decided before by", "Bản trước quyết định bởi"), v.decisionText(w.Before), src+".before"),
		v.fact(v.s("Decided after by", "Bản sau quyết định bởi"), v.decisionText(w.After), src+".after"),
	}
	text := v.s(fmt.Sprintf("Witness %s (%s) is decided %s before and %s after.", w.ID, packetText(w.Packet), v.decisionText(w.Before), v.decisionText(w.After)),
		fmt.Sprintf("Witness %s (%s) được quyết định %s ở bản trước và %s ở bản sau.", w.ID, packetText(w.Packet), v.decisionText(w.Before), v.decisionText(w.After)))
	return ClassDecidingRule, facts, text
}

func refText(r verify.RuleRef, v *view) string {
	if r.Implicit {
		return v.s("implicit deny", "deny mặc định")
	}
	text := r.Action
	if r.Ordinal != nil {
		text += fmt.Sprintf(" · ordinal %d", *r.Ordinal)
	}
	if r.Line != nil {
		text += v.s(fmt.Sprintf(" · line %d", *r.Line), fmt.Sprintf(" · dòng %d", *r.Line))
	}
	return text
}

func (v *view) answerChangeClasses(q questionInfo) (string, []Fact, string) {
	if v.analysis == nil {
		return v.notInArtifact(
			v.s("change classes", "lớp thay đổi"),
			v.s("Change classes are recorded in the analysis record. This comparison artifact proves the difference with witnesses instead — ask about a witness or a deciding rule.",
				"Lớp thay đổi được ghi trong bản ghi analysis. Artifact comparison này chứng minh khác biệt bằng witness — hãy hỏi về witness hoặc rule quyết định."))
	}
	o, ok := v.pickObject(q.folded)
	if !ok {
		return v.notInArtifact(v.s("change classes", "lớp thay đổi"),
			v.s("No ACL object was analyzed in this record.", "Bản ghi này không có đối tượng ACL nào được phân tích."))
	}
	ch := o.obj.ACL.Changes
	base := fmt.Sprintf("analysis.objects[%d].acl.changes", o.index)
	facts := []Fact{
		v.fact("ACL "+o.obj.Name+" total classes", fmt.Sprint(ch.TotalClasses), base+".total_classes"),
		v.fact(v.s("Classes complete", "Lớp thay đổi hoàn tất"), boolText(ch.Complete), base+".complete"),
	}
	if ch.Reason != "" {
		facts = append(facts, v.fact(v.s("Classes stop reason", "Lý do dừng lớp thay đổi"), ch.Reason, base+".reason"))
	}
	var lines []string
	for i, c := range ch.Classes {
		if i >= maxListedClasses {
			break
		}
		value := fmt.Sprintf("%s · %s: %s -> %s: %s · %d %s",
			c.Effect, v.s("before", "trước"), refText(c.Before, v), v.s("after", "sau"), refText(c.After, v), c.Pieces, v.s("pieces", "mảnh"))
		facts = append(facts, v.fact(fmt.Sprintf("%s %d", v.s("Class", "Lớp"), i+1), value, fmt.Sprintf("%s.classes[%d]", base, i)))
		lines = append(lines, fmt.Sprintf("%s (%s -> %s)", c.Effect, refText(c.Before, v), refText(c.After, v)))
	}
	var text string
	switch {
	case ch.TotalClasses == 0 && ch.Complete:
		text = v.s(fmt.Sprintf("ACL %s has no change classes: no behavior difference was found in the modeled scope.", o.obj.Name),
			fmt.Sprintf("ACL %s không có lớp thay đổi nào: không tìm thấy khác biệt hành vi trong phạm vi mô hình.", o.obj.Name))
	case !ch.Complete:
		text = v.s(fmt.Sprintf("ACL %s: the change-class search stopped (%s); only the %d fully decided class(es) are listed, the rest is not characterized.", o.obj.Name, ch.Reason, len(ch.Classes)),
			fmt.Sprintf("ACL %s: tìm kiếm lớp thay đổi đã dừng (%s); chỉ %d lớp đã quyết định trọn vẹn được liệt kê, phần còn lại không được suy diễn.", o.obj.Name, ch.Reason, len(ch.Classes)))
	default:
		text = v.s(fmt.Sprintf("ACL %s has %d change class(es): %s.", o.obj.Name, ch.TotalClasses, strings.Join(lines, "; ")),
			fmt.Sprintf("ACL %s có %d lớp thay đổi: %s.", o.obj.Name, ch.TotalClasses, strings.Join(lines, "; ")))
	}
	return ClassChangeClasses, capFacts(facts), text
}

// partialSource locates the most specific partial record available:
// the comparison delta partial first, then the picked object's compare
// partial, then its change-class partial.
func (v *view) partialSource() (*verify.PartialResult, string) {
	if v.comparison != nil && v.comparison.Report.Partial != nil {
		return v.comparison.Report.Partial, "comparison.report.partial"
	}
	if o, ok := v.pickObject(""); ok {
		if o.obj.ACL.Partial != nil {
			return o.obj.ACL.Partial, fmt.Sprintf("analysis.objects[%d].acl.partial", o.index)
		}
		if o.obj.ACL.Changes.Partial != nil {
			return o.obj.ACL.Changes.Partial, fmt.Sprintf("analysis.objects[%d].acl.changes.partial", o.index)
		}
	}
	return nil, ""
}

func (v *view) answerPartial() (string, []Fact, string) {
	p, base := v.partialSource()
	if p == nil {
		// No partial recorded: report the completion state itself.
		if v.comparison != nil {
			facts := []Fact{
				v.fact(v.s("Search complete", "Tìm kiếm hoàn tất"), boolText(v.comparison.Report.SearchComplete), "comparison.report.search_complete"),
				v.fact(v.s("Verdict", "Kết luận"), v.comparison.Report.Verdict, "comparison.report.verdict"),
			}
			text := v.s("Nothing is left undecided in this artifact: the search completed, so there is no undecided remainder.",
				"Không còn phần nào chưa quyết định trong artifact này: tìm kiếm đã hoàn tất nên không có phần dư chưa quyết định.")
			if !v.comparison.Report.SearchComplete {
				text = v.s("This search did not complete, but the engine recorded no partial split for it, so the undecided part cannot be bounded from the artifact.",
					"Tìm kiếm này chưa hoàn tất, nhưng engine không ghi phần tách partial cho nó nên không thể chặn phần chưa quyết định từ artifact.")
			}
			return ClassPartial, facts, text
		}
		if o, ok := v.pickObject(""); ok {
			facts := []Fact{v.fact("ACL "+o.obj.Name+" search complete", boolText(o.obj.ACL.SearchComplete), fmt.Sprintf("analysis.objects[%d].acl.search_complete", o.index))}
			return ClassPartial, facts, v.s(fmt.Sprintf("ACL %s records no partial split; its search-complete state is %s.", o.obj.Name, boolText(o.obj.ACL.SearchComplete)),
				fmt.Sprintf("ACL %s không ghi phần tách partial; trạng thái hoàn tất tìm kiếm của nó là %s.", o.obj.Name, boolText(o.obj.ACL.SearchComplete)))
		}
		return v.answerCannot()
	}
	facts := []Fact{
		v.fact(v.s("Stopped at stage", "Dừng ở stage"), p.Stage, base+".stage"),
		v.fact(v.s("Leading pairs proven identical", "Cặp đầu đã chứng minh giống hệt"), fmt.Sprint(p.PrefixPairs), base+".prefix_pairs"),
		v.fact(v.s("Pairs proven different", "Cặp đã chứng minh khác nhau"), fmt.Sprint(len(p.DecidedPairs)), base+".decided_pairs"),
	}
	for i, pair := range p.DecidedPairs {
		if i >= maxListedPairs {
			break
		}
		facts = append(facts, v.fact(fmt.Sprintf("%s %d", v.s("Decided pair", "Cặp đã quyết"), i+1),
			fmt.Sprintf("before ordinal %d / after ordinal %d · %d %s", pair.Before, pair.After, pair.Pieces, v.s("pieces", "mảnh")),
			fmt.Sprintf("%s.decided_pairs[%d]", base, i)))
	}
	var undecided []string
	for i, part := range p.Undecided {
		value := part.Direction
		if part.BeforeRules != nil {
			value += fmt.Sprintf(" · before rules %d–%d", part.BeforeRules[0], part.BeforeRules[1])
		}
		if part.AfterRules != nil {
			value += fmt.Sprintf(" · after rules %d–%d", part.AfterRules[0], part.AfterRules[1])
		}
		if part.WorkBoxes > 0 {
			value += fmt.Sprintf(" · %d %s", part.WorkBoxes, v.s("work boxes", "work box"))
		}
		facts = append(facts, v.fact(fmt.Sprintf("%s %d", v.s("Undecided part", "Phần chưa quyết"), i+1), value, fmt.Sprintf("%s.undecided[%d]", base, i)))
		undecided = append(undecided, value)
	}
	text := v.s(fmt.Sprintf("The search stopped at stage %s. Already decided: %d leading rule pair(s) proven identical and %d rule pair(s) proven different. The undecided remainder is only bounded, not characterized: %s.",
		p.Stage, p.PrefixPairs, len(p.DecidedPairs), strings.Join(undecided, "; ")),
		fmt.Sprintf("Tìm kiếm đã dừng ở stage %s. Phần đã quyết định: %d cặp rule đầu đã chứng minh giống hệt và %d cặp rule đã chứng minh khác nhau. Phần dư chưa quyết định chỉ được chặn, không được suy diễn: %s.",
			p.Stage, p.PrefixPairs, len(p.DecidedPairs), strings.Join(undecided, "; ")))
	return ClassPartial, capFacts(facts), text
}

func (v *view) answerBudget() (string, []Fact, string) {
	if v.comparison != nil {
		r := v.comparison.Report
		facts := []Fact{
			v.fact(v.s("Verdict", "Kết luận"), r.Verdict, "comparison.report.verdict"),
			v.fact(v.s("Search complete", "Tìm kiếm hoàn tất"), boolText(r.SearchComplete), "comparison.report.search_complete"),
			v.fact(v.s("Stop reason", "Lý do dừng"), stopReasonText(r.Budget.StopReason), "comparison.report.budget.stop_reason"),
			v.fact(v.s("Operations used", "Operations đã dùng"), fmt.Sprintf("%d / %d", r.Budget.Operations, r.Options.Limits.MaxOperations), "comparison.report.budget.operations"),
			v.fact(v.s("Peak boxes", "Box đỉnh"), fmt.Sprintf("%d / %d", r.Budget.PeakBoxes, r.Options.Limits.MaxBoxes), "comparison.report.budget.peak_boxes"),
			v.fact(v.s("Timing limited", "Bị giới hạn thời gian"), boolText(r.Budget.TimingLimited), "comparison.report.budget.timing_limited"),
		}
		var text string
		if r.SearchComplete {
			text = v.s(fmt.Sprintf("The search completed inside its budget: %d operations and a peak of %d boxes, below the caps of %d and %d.",
				r.Budget.Operations, r.Budget.PeakBoxes, r.Options.Limits.MaxOperations, r.Options.Limits.MaxBoxes),
				fmt.Sprintf("Tìm kiếm đã hoàn tất trong ngân sách: %d operations và đỉnh %d box, dưới các trần %d và %d.",
					r.Budget.Operations, r.Budget.PeakBoxes, r.Options.Limits.MaxOperations, r.Options.Limits.MaxBoxes))
		} else {
			text = v.s(fmt.Sprintf("The search stopped on its budget (stop reason: %s) after %d operations with a peak of %d boxes. A stopped search is never upgraded to a verdict of safety.",
				stopReasonText(r.Budget.StopReason), r.Budget.Operations, r.Budget.PeakBoxes),
				fmt.Sprintf("Tìm kiếm đã dừng vì ngân sách (lý do dừng: %s) sau %d operations với đỉnh %d box. Tìm kiếm bị dừng không bao giờ được nâng thành kết luận an toàn.",
					stopReasonText(r.Budget.StopReason), r.Budget.Operations, r.Budget.PeakBoxes))
		}
		return ClassBudget, facts, text
	}
	if v.analysis != nil && v.analysis.GlobalBudget != nil {
		g := v.analysis.GlobalBudget
		facts := []Fact{
			v.fact(v.s("Global operations", "Operations toàn cục"), fmt.Sprintf("%d / %d", g.Operations, g.MaxOperations), "analysis.global_budget.operations"),
			v.fact(v.s("Global boxes", "Box toàn cục"), fmt.Sprintf("%d / %d", g.Boxes, g.MaxBoxes), "analysis.global_budget.boxes"),
		}
		text := v.s(fmt.Sprintf("The analysis-wide budget ledger bound this run: %d of %d operations and %d of %d boxes were charged across all objects; objects past the ledger are marked not analyzed rather than silently passed.",
			g.Operations, g.MaxOperations, g.Boxes, g.MaxBoxes),
			fmt.Sprintf("Sổ ngân sách toàn cục đã ràng buộc lượt này: %d trên %d operations và %d trên %d box bị tính cho mọi đối tượng; đối tượng vượt sổ được đánh dấu chưa phân tích thay vì bị bỏ qua lặng lẽ.",
				g.Operations, g.MaxOperations, g.Boxes, g.MaxBoxes))
		return ClassBudget, facts, text
	}
	if o, ok := v.pickObject(""); ok {
		b := o.obj.ACL.Budget
		base := fmt.Sprintf("analysis.objects[%d].acl.budget", o.index)
		facts := []Fact{
			v.fact("ACL "+o.obj.Name+" stop reason", stopReasonText(b.StopReason), base+".stop_reason"),
			v.fact("ACL "+o.obj.Name+" operations", fmt.Sprint(b.Operations), base+".operations"),
			v.fact("ACL "+o.obj.Name+" peak boxes", fmt.Sprint(b.PeakBoxes), base+".peak_boxes"),
		}
		text := v.s(fmt.Sprintf("ACL %s used %d operations with a peak of %d boxes; stop reason: %s.", o.obj.Name, b.Operations, b.PeakBoxes, stopReasonText(b.StopReason)),
			fmt.Sprintf("ACL %s đã dùng %d operations với đỉnh %d box; lý do dừng: %s.", o.obj.Name, b.Operations, b.PeakBoxes, stopReasonText(b.StopReason)))
		return ClassBudget, facts, text
	}
	return v.answerCannot()
}

func stopReasonText(reason string) string {
	if reason == "" {
		return "none (completed)"
	}
	return reason
}

func (v *view) answerScope() (string, []Fact, string) {
	var facts []Fact
	if v.comparison != nil {
		c := v.comparison
		facts = append(facts,
			v.fact(v.s("Modeled scope", "Phạm vi mô hình"), c.Scope, "comparison.scope"),
			v.fact(v.s("Engine", "Engine"), c.EngineVersion, "comparison.engine_version"),
			v.fact(v.s("ACL", "ACL"), c.Before.Name, "comparison.before.name"),
		)
	}
	if v.analysis != nil {
		for i, lim := range v.analysis.Limitations {
			if i >= 8 {
				break
			}
			facts = append(facts, v.fact(v.s("Limitation", "Giới hạn"), lim, fmt.Sprintf("analysis.limitations[%d]", i)))
		}
		facts = append(facts, v.fact(v.s("Analysis engine", "Engine analysis"), v.analysis.Versions.ACLEngine, "analysis.versions.acl_engine"))
	}
	text := v.s("Every result in this artifact is stated only within the modeled scope recorded above; behavior outside that scope is not claimed in either direction.",
		"Mọi kết quả trong artifact này chỉ được khẳng định trong phạm vi mô hình đã ghi ở trên; hành vi ngoài phạm vi đó không được khẳng định theo bất kỳ phía nào.")
	return ClassScope, capFacts(facts), text
}

func (v *view) notInArtifact(needed, where string) (string, []Fact, string) {
	kinds := []string{}
	if v.comparison != nil {
		kinds = append(kinds, "comparison")
	}
	if v.analysis != nil {
		kinds = append(kinds, "analysis")
	}
	facts := []Fact{v.fact(v.s("Artifacts present", "Artifact hiện có"), strings.Join(kinds, ", "), "request")}
	text := v.s(fmt.Sprintf("This artifact does not contain %s. %s", needed, where),
		fmt.Sprintf("Artifact này không chứa %s. %s", needed, where))
	return ClassNotInArtifact, facts, text
}

func (v *view) answerCannot() (string, []Fact, string) {
	kinds := []string{}
	if v.comparison != nil {
		kinds = append(kinds, "comparison")
	}
	if v.analysis != nil {
		kinds = append(kinds, "analysis")
	}
	facts := []Fact{
		v.fact(v.s("Artifact digest", "Digest artifact"), v.anchor, "artifact.sha256"),
		v.fact(v.s("Artifacts present", "Artifact hiện có"), strings.Join(kinds, ", "), "request"),
	}
	text := v.s("I can't answer that from this artifact — I only report what the artifact itself records, and I never speculate. I can answer questions about: the verdict, obligation statuses and their witnesses, difference witnesses and deciding rules, change classes, what remains undecided, and why the search stopped.",
		"Tôi không trả lời được câu đó từ artifact này — tôi chỉ báo những gì artifact tự ghi lại và không bao giờ suy đoán. Tôi trả lời được các câu về: kết luận, trạng thái điều kiện và witness của chúng, witness khác biệt và rule quyết định, lớp thay đổi, phần còn chưa quyết định, và lý do tìm kiếm dừng.")
	return ClassCannotAnswer, facts, text
}

// regionText renders an obligation's match region in the compact form
// the brief CLI uses: protocol, source -> destination, destination ports.
func regionText(r intent.Region) string {
	text := r.Protocol + " " + r.SrcCIDR + " -> " + r.DstCIDR
	if len(r.SrcPorts) > 0 {
		text += " src ports " + intervalText(r.SrcPorts)
	}
	if len(r.DstPorts) > 0 {
		text += " dst ports " + intervalText(r.DstPorts)
	}
	return text
}

func intervalText(intervals []intent.Interval) string {
	parts := make([]string, 0, len(intervals))
	for _, iv := range intervals {
		if iv.Lo == iv.Hi {
			parts = append(parts, fmt.Sprint(iv.Lo))
		} else {
			parts = append(parts, fmt.Sprintf("%d-%d", iv.Lo, iv.Hi))
		}
	}
	return strings.Join(parts, ",")
}
